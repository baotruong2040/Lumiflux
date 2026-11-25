package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/truong/discord-clone-api/internal/config"
	"github.com/truong/discord-clone-api/internal/repository/postgres"
	"github.com/truong/discord-clone-api/pkg/idgen"
)

func main() {
	channelFlag := flag.String("channel", "", "Target channel UUID")
	userFlag := flag.String("user", "", "Author user UUID")
	countFlag := flag.Int("count", 1000, "Number of messages to seed")
	prefixFlag := flag.String("prefix", "Seeded message", "Prefix for generated message content")
	flag.Parse()

	channelID, err := uuid.Parse(*channelFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid -channel UUID: %v\n", err)
		os.Exit(1)
	}
	authorID, err := uuid.Parse(*userFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid -user UUID: %v\n", err)
		os.Exit(1)
	}
	if *countFlag < 1 {
		fmt.Fprintln(os.Stderr, "-count must be >= 1")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	baseTime := time.Now().UTC().Add(-time.Duration(*countFlag) * time.Second)
	rows := make([][]any, 0, *countFlag)
	for i := 1; i <= *countFlag; i++ {
		msgID, genErr := idgen.NewUUIDv7()
		if genErr != nil {
			fmt.Fprintf(os.Stderr, "generate UUIDv7: %v\n", genErr)
			os.Exit(1)
		}
		createdAt := baseTime.Add(time.Duration(i-1) * time.Second)
		content := fmt.Sprintf("%s #%04d", *prefixFlag, i)
		rows = append(rows, []any{msgID, channelID, authorID, content, createdAt})
	}

	copied, err := pool.CopyFrom(
		ctx,
		pgx.Identifier{"messages"},
		[]string{"id", "channel_id", "author_id", "content", "created_at"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "copy messages: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Seeded %d messages into channel %s\n", copied, channelID)
}
