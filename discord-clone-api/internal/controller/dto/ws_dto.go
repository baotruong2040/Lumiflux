package dto

// WSTicketResponse represents the JSON body returned by POST /api/v1/ws/ticket.
type WSTicketResponse struct {
	Ticket string `json:"ticket"`
}
