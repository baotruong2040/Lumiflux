package router

import (
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/truong/discord-clone-api/internal/config"
	"github.com/truong/discord-clone-api/internal/controller"
	"github.com/truong/discord-clone-api/internal/domain"
	"github.com/truong/discord-clone-api/internal/router/middleware"
)

// Dependencies bundles the components required to build the HTTP router.
type Dependencies struct {
	Config            *config.Config
	Logger            *slog.Logger
	TokenManager      domain.TokenManager
	RateLimiter       domain.RateLimiter
	HealthController  *controller.HealthController
	AuthController    *controller.AuthController
	UserController    *controller.UserController
	GuildController   *controller.GuildController
	InviteController  *controller.InviteController
	ChannelController  *controller.ChannelController
	MessageController  *controller.MessageController
	WSTicketController *controller.WSTicketController
	WSHandler          gin.HandlerFunc
}

// New constructs and configures the Gin engine with middlewares and v1 routes.
func New(deps Dependencies) *gin.Engine {
	switch strings.ToLower(deps.Config.AppEnv) {
	case "production":
		gin.SetMode(gin.ReleaseMode)
	case "test":
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()
	_ = r.SetTrustedProxies(nil)

	r.Use(
		middleware.RequestID(deps.Logger),
		middleware.Recover(deps.Logger),
		middleware.AccessLog(),
		middleware.CORS(deps.Config.CORSAllowedOrigins),
	)

	r.GET("/healthz", deps.HealthController.Healthz)
	r.GET("/readyz", deps.HealthController.Readyz)
	if deps.WSHandler != nil {
		r.GET("/ws", deps.WSHandler)
	}

	v1 := r.Group("/api/v1")
	auth := v1.Group("/auth")
	{
		auth.POST("/register", deps.AuthController.Register)
		auth.POST(
			"/login",
			middleware.RateLimit(deps.RateLimiter, "rl:login", deps.Config.LoginRateLimit, deps.Config.LoginRateWindow),
			deps.AuthController.Login,
		)
		auth.POST("/refresh", deps.AuthController.Refresh)
		auth.POST("/logout", middleware.Auth(deps.TokenManager), deps.AuthController.Logout)
	}

	users := v1.Group("/users", middleware.Auth(deps.TokenManager))
	{
		users.GET("/@me", deps.UserController.GetMe)
		users.PATCH("/@me", deps.UserController.UpdateMe)
	}

	if deps.WSTicketController != nil {
		ws := v1.Group("/ws", middleware.Auth(deps.TokenManager))
		{
			ws.POST("/ticket", deps.WSTicketController.Create)
		}
	}

	if deps.GuildController != nil {
		guilds := v1.Group("/guilds", middleware.Auth(deps.TokenManager))
		{
			guilds.POST("", deps.GuildController.Create)
			guilds.GET("", deps.GuildController.List)
			guilds.GET("/:guildId", deps.GuildController.Get)
			guilds.PATCH("/:guildId", deps.GuildController.Update)
			guilds.DELETE("/:guildId", deps.GuildController.Delete)
			guilds.GET("/:guildId/members", deps.GuildController.ListMembers)
			guilds.DELETE("/:guildId/members/@me", deps.GuildController.Leave)
			if deps.InviteController != nil {
				guilds.POST("/:guildId/invites", deps.InviteController.Create)
			}
			if deps.ChannelController != nil {
				guilds.GET("/:guildId/channels", deps.ChannelController.List)
				guilds.POST("/:guildId/channels", deps.ChannelController.Create)
			}
		}
	}

	if deps.InviteController != nil {
		invites := v1.Group("/invites", middleware.Auth(deps.TokenManager))
		{
			invites.POST("/:code/accept", deps.InviteController.Accept)
		}
	}

	if deps.ChannelController != nil || deps.MessageController != nil {
		channels := v1.Group("/channels", middleware.Auth(deps.TokenManager))
		{
			if deps.ChannelController != nil {
				channels.PATCH("/:channelId", deps.ChannelController.Update)
				channels.DELETE("/:channelId", deps.ChannelController.Delete)
			}
			if deps.MessageController != nil {
				channels.GET("/:channelId/messages", deps.MessageController.List)
				channels.POST("/:channelId/messages", deps.MessageController.Send)
				channels.PATCH("/:channelId/messages/:messageId", deps.MessageController.Update)
				channels.DELETE("/:channelId/messages/:messageId", deps.MessageController.Delete)
			}
		}
	}

	return r
}
