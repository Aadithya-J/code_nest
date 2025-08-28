package api

import (
	"net/http"

	"github.com/Aadithya-J/code_nest/services/auth-service/internal/service"
	"github.com/Aadithya-J/code_nest/proto"
	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for authentication
type Handler struct {
	authService *service.AuthService
}

// NewHandler creates a new Handler
func NewHandler(authSvc *service.AuthService) *Handler {
	return &Handler{authService: authSvc}
}

func (h *Handler) Signup(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	signupReq := &proto.SignupRequest{
		Email:    req.Email,
		Password: req.Password,
	}
	token, err := h.authService.Signup(c.Request.Context(), signupReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	loginReq := &proto.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	}
	token, err := h.authService.Login(c.Request.Context(), loginReq)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) GoogleLogin(c *gin.Context) {
	// generate state (in production, store and verify state)
	state := "state"
	req := &proto.GetGoogleAuthURLRequest{State: state}
	urlResp, err := h.authService.GetGoogleAuthURL(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, urlResp.Url)
}

func (h *Handler) GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code query param required"})
		return
	}
	req := &proto.HandleGoogleCallbackRequest{Code: code}
	token, err := h.authService.HandleGoogleCallback(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}
