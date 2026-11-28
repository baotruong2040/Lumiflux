package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/truong/discord-clone-api/internal/config"
)

type authResult struct {
	AccessToken string `json:"accessToken"`
	User        struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

type wsClientHandle struct {
	conn   *websocket.Conn
	userID string
}

func doJSON(client *http.Client, method, url, token string, body any, wantStatus int, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		return fmt.Errorf("%s %s -> status %d (want %d)", method, url, resp.StatusCode, wantStatus)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func main() {
	apiURL := flag.String("api", "http://127.0.0.1:8080", "Base HTTP API URL")
	wsURL := flag.String("ws", "ws://127.0.0.1:8080", "Base WebSocket URL")
	numClients := flag.Int("clients", 10, "Number of concurrent WebSocket clients")
	numMessages := flag.Int("messages", 20, "Number of broadcast messages to send")
	flag.Parse()

	if *numClients < 1 || *numMessages < 1 {
		fmt.Fprintf(os.Stderr, "clients and messages must be >= 1\n")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	apiBase := strings.TrimRight(*apiURL, "/")
	wsBase := strings.TrimRight(*wsURL, "/")
	if !strings.HasSuffix(wsBase, "/ws") {
		wsBase += "/ws"
	}

	userCount := min(*numClients, 10)
	runTag := fmt.Sprintf("lt%d_%d", *numClients, time.Now().UnixNano()%1000000)

	users := make([]authResult, userCount)
	userIDs := make([]string, 0, userCount)
	for i := range userCount {
		uname := fmt.Sprintf("%s_%d", runTag, i)
		if err := doJSON(httpClient, http.MethodPost, apiBase+"/api/v1/auth/register", "", map[string]any{
			"username":    uname,
			"email":       uname + "@example.com",
			"displayName": "Loadtest " + uname,
			"password":    "LoadtestPass123!",
		}, http.StatusCreated, &users[i]); err != nil {
			fmt.Fprintf(os.Stderr, "register user %d: %v\n", i, err)
			os.Exit(1)
		}
		userIDs = append(userIDs, users[i].User.ID)
	}

	var createdGuildID string
	defer func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if pool, err := pgxpool.New(cleanCtx, cfg.DatabaseURL); err == nil {
			if createdGuildID != "" {
				_, _ = pool.Exec(cleanCtx, `DELETE FROM guilds WHERE id = $1`, createdGuildID)
			}
			for _, id := range userIDs {
				_, _ = pool.Exec(cleanCtx, `DELETE FROM users WHERE id = $1`, id)
			}
			pool.Close()
		}
		rc := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
		for _, id := range userIDs {
			_ = rc.Del(cleanCtx, "presence:"+id, "rl:"+id+":msg").Err()
		}
		_ = rc.Close()
	}()

	var guild struct {
		ID string `json:"id"`
	}
	if err := doJSON(httpClient, http.MethodPost, apiBase+"/api/v1/guilds", users[0].AccessToken, map[string]any{
		"name": "Loadtest Guild " + runTag,
	}, http.StatusCreated, &guild); err != nil {
		fmt.Fprintf(os.Stderr, "create guild: %v\n", err)
		os.Exit(1)
	}
	createdGuildID = guild.ID

	var channels []struct {
		ID string `json:"id"`
	}
	if err := doJSON(httpClient, http.MethodGet, apiBase+"/api/v1/guilds/"+guild.ID+"/channels", users[0].AccessToken, nil, http.StatusOK, &channels); err != nil || len(channels) == 0 {
		fmt.Fprintf(os.Stderr, "list channels: %v\n", err)
		os.Exit(1)
	}
	channelID := channels[0].ID

	var inv struct {
		Code string `json:"code"`
	}
	if err := doJSON(httpClient, http.MethodPost, apiBase+"/api/v1/guilds/"+guild.ID+"/invites", users[0].AccessToken, map[string]any{}, http.StatusCreated, &inv); err != nil {
		fmt.Fprintf(os.Stderr, "create invite: %v\n", err)
		os.Exit(1)
	}
	for i := 1; i < len(users); i++ {
		if err := doJSON(httpClient, http.MethodPost, apiBase+"/api/v1/invites/"+inv.Code+"/accept", users[i].AccessToken, nil, http.StatusOK, nil); err != nil {
			fmt.Fprintf(os.Stderr, "accept invite %d: %v\n", i, err)
			os.Exit(1)
		}
	}

	// Open N concurrent WebSocket connections (throttled dialer concurrency).
	handles := make([]wsClientHandle, *numClients)
	sem := make(chan struct{}, 32)
	var dialWG sync.WaitGroup
	var dialErrMu sync.Mutex
	var firstDialErr error

	for i := range *numClients {
		dialWG.Add(1)
		go func(idx int) {
			defer dialWG.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			u := users[idx%len(users)]
			var ticket struct {
				Ticket string `json:"ticket"`
			}
			if err := doJSON(httpClient, http.MethodPost, apiBase+"/api/v1/ws/ticket", u.AccessToken, nil, http.StatusOK, &ticket); err != nil {
				dialErrMu.Lock()
				if firstDialErr == nil {
					firstDialErr = err
				}
				dialErrMu.Unlock()
				return
			}
			dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
			conn, resp, err := dialer.Dial(wsBase+"?ticket="+ticket.Ticket, http.Header{"Origin": {"http://127.0.0.1:5173"}})
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err != nil {
				dialErrMu.Lock()
				if firstDialErr == nil {
					firstDialErr = err
				}
				dialErrMu.Unlock()
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			_, raw, err := conn.ReadMessage()
			if err != nil {
				_ = conn.Close()
				dialErrMu.Lock()
				if firstDialErr == nil {
					firstDialErr = err
				}
				dialErrMu.Unlock()
				return
			}
			var readyEnv struct {
				T string `json:"t"`
			}
			if err := json.Unmarshal(raw, &readyEnv); err != nil || readyEnv.T != "READY" {
				_ = conn.Close()
				dialErrMu.Lock()
				if firstDialErr == nil {
					firstDialErr = fmt.Errorf("expected READY, got %s", string(raw))
				}
				dialErrMu.Unlock()
				return
			}
			_ = conn.SetReadDeadline(time.Time{})
			handles[idx] = wsClientHandle{conn: conn, userID: u.User.ID}
		}(i)
	}
	dialWG.Wait()
	if firstDialErr != nil {
		fmt.Fprintf(os.Stderr, "dial websocket clients: %v\n", firstDialErr)
		os.Exit(1)
	}

	defer func() {
		for _, h := range handles {
			if h.conn != nil {
				_ = h.conn.Close()
			}
		}
	}()

	// Small settle pause so async presence/pubsub initial frames settle cleanly.
	time.Sleep(150 * time.Millisecond)

	var sentMu sync.Mutex
	sentAtByNonce := make(map[string]time.Time, *numMessages)
	totalExpected := *numClients * *numMessages

	var latMu sync.Mutex
	latenciesMs := make([]float64, 0, totalExpected)
	doneCh := make(chan struct{})
	var doneOnce sync.Once

	var readerWG sync.WaitGroup
	for idx, h := range handles {
		readerWG.Add(1)
		go func(clientIdx int, conn *websocket.Conn) {
			defer readerWG.Done()
			seenNonces := make(map[string]struct{}, *numMessages)
			for {
				_, raw, err := conn.ReadMessage()
				if err != nil {
					return
				}
				var frame struct {
					T string          `json:"t"`
					D json.RawMessage `json:"d"`
				}
				if json.Unmarshal(raw, &frame) != nil || frame.T != "MESSAGE_CREATE" {
					continue
				}
				var msg struct {
					Nonce *string `json:"nonce"`
				}
				if json.Unmarshal(frame.D, &msg) != nil || msg.Nonce == nil {
					continue
				}
				nonce := *msg.Nonce
				if !strings.HasPrefix(nonce, runTag) {
					continue
				}
				if _, dup := seenNonces[nonce]; dup {
					continue
				}
				seenNonces[nonce] = struct{}{}

				sentMu.Lock()
				sentAt, ok := sentAtByNonce[nonce]
				sentMu.Unlock()
				if !ok {
					continue
				}
				rttMs := float64(time.Since(sentAt).Microseconds()) / 1000.0
				latMu.Lock()
				latenciesMs = append(latenciesMs, rttMs)
				if len(latenciesMs) >= totalExpected {
					doneOnce.Do(func() { close(doneCh) })
				}
				latMu.Unlock()
			}
		}(idx, h.conn)
	}

	for m := range *numMessages {
		nonce := fmt.Sprintf("%s-%s-%04d", runTag, uuid.NewString()[:8], m)
		sender := users[m%len(users)]

		sentMu.Lock()
		sentAtByNonce[nonce] = time.Now()
		sentMu.Unlock()

		if err := doJSON(httpClient, http.MethodPost, apiBase+"/api/v1/channels/"+channelID+"/messages", sender.AccessToken, map[string]any{
			"content": fmt.Sprintf("loadtest message %d", m),
			"nonce":   nonce,
		}, http.StatusCreated, nil); err != nil {
			fmt.Fprintf(os.Stderr, "send message %d: %v\n", m, err)
			os.Exit(1)
		}
		time.Sleep(15 * time.Millisecond)
	}

	select {
	case <-doneCh:
	case <-time.After(20 * time.Second):
		latMu.Lock()
		got := len(latenciesMs)
		latMu.Unlock()
		fmt.Fprintf(os.Stderr, "timed out waiting for deliveries: got %d / %d\n", got, totalExpected)
		os.Exit(1)
	}

	for _, h := range handles {
		if h.conn != nil {
			_ = h.conn.Close()
		}
	}
	readerWG.Wait()

	latMu.Lock()
	sort.Float64s(latenciesMs)
	p50 := percentile(latenciesMs, 50)
	p95 := percentile(latenciesMs, 95)
	p99 := percentile(latenciesMs, 99)
	minV := latenciesMs[0]
	maxV := latenciesMs[len(latenciesMs)-1]
	delivered := len(latenciesMs)
	latMu.Unlock()

	fmt.Printf("LOADTEST clients=%d messages=%d delivered=%d/%d p50=%.2fms p95=%.2fms p99=%.2fms min=%.2fms max=%.2fms\n",
		*numClients, *numMessages, delivered, totalExpected, p50, p95, p99, minV, maxV)
}
