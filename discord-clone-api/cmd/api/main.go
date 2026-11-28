package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/truong/discord-clone-api/internal/config"
	"github.com/truong/discord-clone-api/internal/controller"
	"github.com/truong/discord-clone-api/internal/gateway"
	"github.com/truong/discord-clone-api/internal/repository/postgres"
	"github.com/truong/discord-clone-api/internal/repository/redisstore"
	"github.com/truong/discord-clone-api/internal/router"
	"github.com/truong/discord-clone-api/internal/router/middleware"
	"github.com/truong/discord-clone-api/internal/service"
	"github.com/truong/discord-clone-api/pkg/hasher"
	"github.com/truong/discord-clone-api/pkg/jwtutil"
	"github.com/truong/discord-clone-api/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log := logger.New("error", "startup")
		log.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel, cfg.AppEnv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pgPool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect to postgres", "err", err)
		os.Exit(1)
	}
	defer pgPool.Close()

	redisClient, err := redisstore.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Error("failed to connect to redis", "err", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			log.Warn("failed to close redis client", "err", closeErr)
		}
	}()

	userRepo := postgres.NewUserRepository(pgPool)
	guildRepo := postgres.NewGuildRepository(pgPool)
	channelRepo := postgres.NewChannelRepository(pgPool)
	messageRepo := postgres.NewMessageRepository(pgPool)
	inviteRepo := postgres.NewInviteRepository(pgPool)
	tokenStore := redisstore.NewTokenStore(redisClient)
	ticketStore := redisstore.NewTicketStore(redisClient)
	presenceStore := redisstore.NewPresenceStore(redisClient)
	rateLimiter := redisstore.NewRateLimiter(redisClient)
	pgHealth := postgres.NewHealthChecker(pgPool)
	redisHealth := redisstore.NewHealthChecker(redisClient)

	pwHasher := hasher.NewBcryptHasher(cfg.BcryptCost)
	jwtMgr := jwtutil.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	hub := gateway.NewHub(cfg.WSHeartbeatInterval, log)
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		hub.Run(ctx)
	}()

	eventPublisher := redisstore.NewEventPublisher(redisClient, log)
	subscriber := gateway.NewSubscriber(redisClient, hub, log)
	subDone, err := subscriber.Start(ctx)
	if err != nil {
		log.Error("failed to start redis pubsub subscriber", "err", err)
		stop()
		<-hubDone
		os.Exit(1)
	}

	authSvc := service.NewAuthService(userRepo, tokenStore, pwHasher, jwtMgr, cfg.RefreshTokenTTL)
	guildSvc := service.NewGuildService(guildRepo, presenceStore, hub, eventPublisher)
	inviteSvc := service.NewInviteService(guildRepo, inviteRepo, hub, eventPublisher)
	channelSvc := service.NewChannelService(guildRepo, channelRepo, eventPublisher)
	messageSvc := service.NewMessageService(
		guildRepo,
		channelRepo,
		messageRepo,
		rateLimiter,
		eventPublisher,
		cfg.MessageRateLimit,
		cfg.MessageRateWindow,
	)
	ticketSvc := service.NewTicketService(ticketStore, cfg.WSTicketTTL)
	wsHandler := gateway.NewHandler(hub, ticketSvc, guildRepo, cfg.CORSAllowedOrigins, log)

	engine := router.New(router.Dependencies{
		Config:             cfg,
		Logger:             log,
		TokenManager:       jwtMgr,
		RateLimiter:        rateLimiter,
		HealthController:   controller.NewHealthController(pgHealth, redisHealth, log),
		AuthController:     controller.NewAuthController(authSvc),
		UserController:     controller.NewUserController(authSvc, middleware.GetUserID),
		GuildController:    controller.NewGuildController(guildSvc, middleware.GetUserID),
		InviteController:   controller.NewInviteController(inviteSvc, middleware.GetUserID),
		ChannelController:  controller.NewChannelController(channelSvc, middleware.GetUserID),
		MessageController:  controller.NewMessageController(messageSvc, middleware.GetUserID),
		WSTicketController: controller.NewWSTicketController(ticketSvc, middleware.GetUserID),
		WSHandler:          wsHandler.ServeGin,
	})

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api server listening", "addr", cfg.Addr())
		if serveErr := srv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case serveErr := <-errCh:
		stop()
		<-subDone
		<-hubDone
		if serveErr != nil {
			log.Error("http server error", "err", serveErr)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	select {
	case <-subDone:
	case <-shutdownCtx.Done():
		log.Warn("timed out waiting for redis subscriber shutdown")
	}
	select {
	case <-hubDone:
	case <-shutdownCtx.Done():
		log.Warn("timed out waiting for websocket hub shutdown")
	}
	log.Info("api server stopped cleanly")
}
