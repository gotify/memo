package auth

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/gotify/server/v2/auth/password"
	"github.com/gotify/server/v2/model"
)

const (
	headerName = "X-Gotify-Key"
)

// The Database interface for encapsulating database access.
type Database interface {
	GetApplicationByToken(token string) (*model.Application, error)
	GetClientByToken(token string) (*model.Client, error)
	GetPluginConfByToken(token string) (*model.PluginConf, error)
	GetUserByName(name string) (*model.User, error)
	GetUserByID(id uint) (*model.User, error)
}

// Auth is the provider for authentication middleware.
type Auth struct {
	DB Database
}

type authenticate func(tokenID string, user *model.User) (authenticated, success bool, userId uint, err error)

// RequireAdmin returns a gin middleware which requires a client token or basic authentication header to be supplied
// with the request. Also the authenticated user must be an administrator.
func (a *Auth) RequireAdmin() gin.HandlerFunc {
	return a.requireToken(func(tokenID string, user *model.User) (bool, bool, uint, error) {
		if user != nil {
			return true, user.Admin, user.ID, nil
		}
		if token, err := a.DB.GetClientByToken(tokenID); err != nil {
			return false, false, 0, err
		} else if token != nil {
			user, err := a.DB.GetUserByID(token.UserID)
			if err != nil {
				return false, false, token.UserID, err
			}
			return true, user.Admin, token.UserID, nil
		}
		return false, false, 0, nil
	})
}

// RequireClient returns a gin middleware which requires a client token or basic authentication header to be supplied
// with the request.
func (a *Auth) RequireClient() gin.HandlerFunc {
	return a.requireToken(func(tokenID string, user *model.User) (bool, bool, uint, error) {
		if user != nil {
			return true, true, user.ID, nil
		}
		if token, err := a.DB.GetClientByToken(tokenID); err != nil {
			return false, false, 0, err
		} else if token != nil {
			return true, true, token.UserID, nil
		}
		return false, false, 0, nil
	})
}

// RequireApplicationToken returns a gin middleware which requires an application token to be supplied with the request.
func (a *Auth) RequireApplicationToken() gin.HandlerFunc {
	return a.requireToken(func(tokenID string, user *model.User) (bool, bool, uint, error) {
		if user != nil {
			return true, false, 0, nil
		}
		if token, err := a.DB.GetApplicationByToken(tokenID); err != nil {
			return false, false, 0, err
		} else if token != nil {
			return true, true, token.UserID, nil
		}
		return false, false, 0, nil
	})
}

func (a *Auth) tokenFromQueryOrHeader(ctx *gin.Context) string {
	if token := a.tokenFromQuery(ctx); token != "" {
		return token
	} else if token := a.tokenFromHeader(ctx); token != "" {
		return token
	}
	return ""
}

func (a *Auth) tokenFromQuery(ctx *gin.Context) string {
	return ctx.Request.URL.Query().Get("token")
}

func (a *Auth) tokenFromHeader(ctx *gin.Context) string {
	return ctx.Request.Header.Get(headerName)
}

func (a *Auth) userFromBasicAuth(ctx *gin.Context) (*model.User, error) {
	if name, pass, ok := ctx.Request.BasicAuth(); ok {
		if user, err := a.DB.GetUserByName(name); err != nil {
			return nil, err
		} else if user != nil && password.ComparePassword(user.Pass, []byte(pass)) {
			return user, nil
		}
	}
	return nil, nil
}

func (a *Auth) requireToken(auth authenticate) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := a.tokenFromQueryOrHeader(ctx)
		user, err := a.userFromBasicAuth(ctx)
		if err != nil {
			ctx.AbortWithError(500, errors.New("an error occurred while authenticating user"))
			return
		}

		if user != nil || token != "" {
			authenticated, ok, userID, err := auth(token, user)
			if err != nil {
				ctx.AbortWithError(500, errors.New("an error occurred while authenticating user"))
				return
			} else if ok {
				RegisterAuthentication(ctx, user, userID, token)
				ctx.Next()
				return
			} else if authenticated {
				ctx.AbortWithError(403, errors.New("you are not allowed to access this api"))
				return
			}
		}
		ctx.AbortWithError(401, errors.New("you need to provide a valid access token or user credentials to access this api"))
	}
}

func (a *Auth) Optional() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token := a.tokenFromQueryOrHeader(ctx)
		user, err := a.userFromBasicAuth(ctx)
		if err != nil {
			RegisterAuthentication(ctx, nil, 0, "")
			ctx.Next()
			return
		}

		if user != nil {
			RegisterAuthentication(ctx, user, user.ID, token)
			ctx.Next()
			return
		} else if token != "" {
			if tokenClient, err := a.DB.GetClientByToken(token); err == nil && tokenClient != nil {
				RegisterAuthentication(ctx, user, tokenClient.UserID, token)
				ctx.Next()
				return
			}
		}
		RegisterAuthentication(ctx, nil, 0, "")
		ctx.Next()
	}
}
