package api

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/gotify/location"
	"github.com/gotify/server/auth"
	"github.com/gotify/server/model"
)

// The MessageDatabase interface for encapsulating database access.
type MessageDatabase interface {
	GetMessagesByApplication(appID uint) ([]*model.Message, error)
	GetMessagesByApplicationSince(appID uint, limit int, since uint) ([]*model.Message, error)
	GetApplicationByID(id uint) (*model.Application, error)
	GetMessagesByUser(userID uint) ([]*model.Message, error)
	GetMessagesByUserSince(userID uint, limit int, since uint) ([]*model.Message, error)
	DeleteMessageByID(id uint) error
	GetMessageByID(id uint) (*model.Message, error)
	DeleteMessagesByUser(userID uint) error
	DeleteMessagesByApplication(applicationID uint) error
	CreateMessage(message *model.Message) error
	GetApplicationByToken(token string) (*model.Application, error)
}

// Notifier notifies when a new message was created.
type Notifier interface {
	Notify(userID uint, event model.Event)
}

// The MessageAPI provides handlers for managing messages.
type MessageAPI struct {
	DB       MessageDatabase
	Notifier Notifier
}

type pagingParams struct {
	Limit int  `form:"limit" binding:"min=1,max=200"`
	Since uint `form:"since" binding:"min=0"`
}

// GetMessages returns all messages from a user.
// swagger:operation GET /message message getMessages
//
// Return all messages.
//
// ---
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// parameters:
// - name: limit
//   in: query
//   description: the maximal amount of messages to return
//   required: false
//   maximum: 200
//   minimum: 1
//   default: 100
//   type: integer
// - name: since
//   in: query
//   description: return all messages with an ID less than this value
//   minimum: 0
//   required: false
//   type: integer
// responses:
//   200:
//     description: Ok
//     schema:
//         $ref: "#/definitions/PagedMessages"
//   400:
//     description: Bad Request
//     schema:
//         $ref: "#/definitions/Error"
//   401:
//     description: Unauthorized
//     schema:
//         $ref: "#/definitions/Error"
//   403:
//     description: Forbidden
//     schema:
//         $ref: "#/definitions/Error"
func (a *MessageAPI) GetMessages(ctx *gin.Context) {
	userID := auth.GetUserID(ctx)
	withPaging(ctx, func(params *pagingParams) {
		// the +1 is used to check if there are more messages and will be removed on buildWithPaging
		messages, err := a.DB.GetMessagesByUserSince(userID, params.Limit+1, params.Since)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		ctx.JSON(200, buildWithPaging(ctx, params, messages))
	})
}

func buildWithPaging(ctx *gin.Context, paging *pagingParams, messages []*model.Message) *model.PagedMessages {
	next := ""
	since := uint(0)
	useMessages := messages
	if len(messages) > paging.Limit {
		useMessages = messages[:len(messages)-1]
		since = useMessages[len(useMessages)-1].ID
		url := location.Get(ctx)
		url.Path = ctx.Request.URL.Path
		query := url.Query()
		query.Add("limit", strconv.Itoa(paging.Limit))
		query.Add("since", strconv.FormatUint(uint64(since), 10))
		url.RawQuery = query.Encode()
		next = url.String()
	}
	return &model.PagedMessages{
		Paging:   model.Paging{Size: len(useMessages), Limit: paging.Limit, Next: next, Since: since},
		Messages: toExternalMessages(useMessages),
	}
}

func withPaging(ctx *gin.Context, f func(pagingParams *pagingParams)) {
	params := &pagingParams{Limit: 100}
	if err := ctx.MustBindWith(params, binding.Query); err == nil {
		f(params)
	}
}

// GetMessagesWithApplication returns all messages from a specific application.
// swagger:operation GET /application/{id}/message message getAppMessages
//
// Return all messages from a specific application.
//
// ---
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// parameters:
// - name: id
//   in: path
//   description: the application id
//   required: true
//   type: integer
// - name: limit
//   in: query
//   description: the maximal amount of messages to return
//   required: false
//   maximum: 200
//   minimum: 1
//   default: 100
//   type: integer
// - name: since
//   in: query
//   description: return all messages with an ID less than this value
//   minimum: 0
//   required: false
//   type: integer
// responses:
//   200:
//     description: Ok
//     schema:
//         $ref: "#/definitions/PagedMessages"
//   400:
//     description: Bad Request
//     schema:
//         $ref: "#/definitions/Error"
//   401:
//     description: Unauthorized
//     schema:
//         $ref: "#/definitions/Error"
//   403:
//     description: Forbidden
//     schema:
//         $ref: "#/definitions/Error"
//   404:
//     description: Not Found
//     schema:
//         $ref: "#/definitions/Error"
func (a *MessageAPI) GetMessagesWithApplication(ctx *gin.Context) {
	withID(ctx, "id", func(id uint) {
		withPaging(ctx, func(params *pagingParams) {
			app, err := a.DB.GetApplicationByID(id)
			if success := successOrAbort(ctx, 500, err); !success {
				return
			}
			if app != nil && app.UserID == auth.GetUserID(ctx) {
				// the +1 is used to check if there are more messages and will be removed on buildWithPaging
				messages, err := a.DB.GetMessagesByApplicationSince(id, params.Limit+1, params.Since)
				if success := successOrAbort(ctx, 500, err); !success {
					return
				}
				ctx.JSON(200, buildWithPaging(ctx, params, messages))
			} else {
				ctx.AbortWithError(404, errors.New("application does not exist"))
			}
		})
	})
}

// DeleteMessages delete all messages from a user.
// swagger:operation DELETE /message message deleteMessages
//
// Delete all messages.
//
// ---
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// responses:
//   200:
//     description: Ok
//   401:
//     description: Unauthorized
//     schema:
//         $ref: "#/definitions/Error"
//   403:
//     description: Forbidden
//     schema:
//         $ref: "#/definitions/Error"
func (a *MessageAPI) DeleteMessages(ctx *gin.Context) {
	userID := auth.GetUserID(ctx)
	messages, err := a.DB.GetMessagesByUser(userID)
	if success := successOrAbort(ctx, 500, err); !success {
		return
	}
	event := &model.MessageDeletions{
		Messages: messages,
	}
	a.Notifier.Notify(auth.GetUserID(ctx), event)
	successOrAbort(ctx, 500, a.DB.DeleteMessagesByUser(userID))
}

// DeleteMessageWithApplication deletes all messages from a specific application.
// swagger:operation DELETE /application/{id}/message message deleteAppMessages
//
// Delete all messages from a specific application.
//
// ---
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// parameters:
// - name: id
//   in: path
//   description: the application id
//   required: true
//   type: integer
// responses:
//   200:
//     description: Ok
//   400:
//     description: Bad Request
//     schema:
//         $ref: "#/definitions/Error"
//   401:
//     description: Unauthorized
//     schema:
//         $ref: "#/definitions/Error"
//   403:
//     description: Forbidden
//     schema:
//         $ref: "#/definitions/Error"
//   404:
//     description: Not Found
//     schema:
//         $ref: "#/definitions/Error"
func (a *MessageAPI) DeleteMessageWithApplication(ctx *gin.Context) {
	withID(ctx, "id", func(id uint) {
		application, err := a.DB.GetApplicationByID(id)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		if application != nil && application.UserID == auth.GetUserID(ctx) {
			messages, err := a.DB.GetMessagesByApplication(id)
			if success := successOrAbort(ctx, 500, err); !success {
				return
			}
			event := &model.MessageDeletions{
				Messages: messages,
			}
			a.Notifier.Notify(auth.GetUserID(ctx), event)
			successOrAbort(ctx, 500, a.DB.DeleteMessagesByApplication(id))
		} else {
			ctx.AbortWithError(404, errors.New("application does not exists"))
		}
	})
}

// DeleteMessage deletes a message with an id.
// swagger:operation DELETE /message/{id} message deleteMessage
//
// Deletes a message with an id.
//
// ---
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// parameters:
// - name: id
//   in: path
//   description: the message id
//   required: true
//   type: integer
// responses:
//   200:
//     description: Ok
//   400:
//     description: Bad Request
//     schema:
//         $ref: "#/definitions/Error"
//   401:
//     description: Unauthorized
//     schema:
//         $ref: "#/definitions/Error"
//   403:
//     description: Forbidden
//     schema:
//         $ref: "#/definitions/Error"
//   404:
//     description: Not Found
//     schema:
//         $ref: "#/definitions/Error"
func (a *MessageAPI) DeleteMessage(ctx *gin.Context) {
	withID(ctx, "id", func(id uint) {
		msg, err := a.DB.GetMessageByID(id)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		if msg == nil {
			ctx.AbortWithError(404, errors.New("message does not exist"))
			return
		}
		app, err := a.DB.GetApplicationByID(msg.ApplicationID)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		if app != nil && app.UserID == auth.GetUserID(ctx) {
			event := &model.MessageDeletions{
				Messages: []*model.Message {msg},
			}
			a.Notifier.Notify(auth.GetUserID(ctx), event)
			successOrAbort(ctx, 500, a.DB.DeleteMessageByID(id))
		} else {
			ctx.AbortWithError(404, errors.New("message does not exist"))
		}
	})
}

// CreateMessage creates a message, authentication via application-token is required.
// swagger:operation POST /message message createMessage
//
// Create a message.
//
// __NOTE__: This API ONLY accepts an application token as authentication.
// ---
// consumes: [application/json]
// produces: [application/json]
// security: [appTokenHeader: [], appTokenQuery: []]
// parameters:
// - name: body
//   in: body
//   description: the message to add
//   required: true
//   schema:
//     $ref: "#/definitions/Message"
// responses:
//   200:
//     description: Ok
//     schema:
//       $ref: "#/definitions/Message"
//   400:
//     description: Bad Request
//     schema:
//         $ref: "#/definitions/Error"
//   401:
//     description: Unauthorized
//     schema:
//         $ref: "#/definitions/Error"
//   403:
//     description: Forbidden
//     schema:
//         $ref: "#/definitions/Error"
func (a *MessageAPI) CreateMessage(ctx *gin.Context) {
	appMessage := model.ApplicationMessage{}
	if err := ctx.Bind(&appMessage); err != nil {
		return
	}
	application, err := a.DB.GetApplicationByToken(auth.GetTokenID(ctx))
	if success := successOrAbort(ctx, 500, err); !success {
		return
	}
	message := appMessage.ToInternal(application.ID)
	if strings.TrimSpace(message.Title) == "" {
		message.Title = application.Name
	}
	if success := successOrAbort(ctx, 500, a.DB.CreateMessage(message)); !success {
		return
	}
	a.Notifier.Notify(auth.GetUserID(ctx), message)
	ctx.JSON(200, message.ToExternal())
}

func toExternalMessages(msg []*model.Message) []*model.MessageExternal {
	res := make([]*model.MessageExternal, len(msg))
	for i := range msg {
		res[i] = msg[i].ToExternal().(*model.MessageExternal)
	}
	return res
}
