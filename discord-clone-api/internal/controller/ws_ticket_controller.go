package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/truong/discord-clone-api/internal/controller/dto"
)

// WSTicketUseCase defines service operations required by WSTicketController.
type WSTicketUseCase interface {
	IssueTicket(ctx context.Context, userID uuid.UUID) (string, error)
}

// WSTicketController handles WebSocket single-use ticket issuance.
type WSTicketController struct {
	tickets       WSTicketUseCase
	extractUserID UserIDExtractor
}

// NewWSTicketController constructs a WSTicketController.
func NewWSTicketController(tickets WSTicketUseCase, extractUserID UserIDExtractor) *WSTicketController {
	return &WSTicketController{
		tickets:       tickets,
		extractUserID: extractUserID,
	}
}

// Create handles POST /api/v1/ws/ticket.
func (ctrl *WSTicketController) Create(c *gin.Context) {
	userID, ok := ctrl.extractUserID(c)
	if !ok {
		WriteErrorStatus(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	ticket, err := ctrl.tickets.IssueTicket(c.Request.Context(), userID)
	if err != nil {
		WriteError(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.WSTicketResponse{Ticket: ticket})
}
