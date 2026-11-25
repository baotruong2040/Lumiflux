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
	Config           *config.Config
	Logger           *slog.Logger
	TokenManager     domain.TokenManager
	RateLimiter      domain.RateLimiter
	HealthController *controller.HealthController
	AuthController   *controller.AuthController
	UserController   *controller.UserController
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

	return r
}
