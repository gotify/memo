package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gotify/server/v2/auth"
	"github.com/gotify/server/v2/auth/password"
	"github.com/gotify/server/v2/model"
)

// The UserDatabase interface for encapsulating database access.
type UserDatabase interface {
	GetUsers() ([]*model.User, error)
	GetUserByID(id uint) (*model.User, error)
	GetUserByName(name string) (*model.User, error)
	DeleteUserByID(id uint) error
	UpdateUser(user *model.User) error
	CreateUser(user *model.User) error
	CountUser(condition ...interface{}) (int, error)
}

// UserChangeNotifier notifies listeners for user changes.
type UserChangeNotifier struct {
	userDeletedCallbacks []func(uid uint) error
	userAddedCallbacks   []func(uid uint) error
}

// OnUserDeleted is called on user deletion.
func (c *UserChangeNotifier) OnUserDeleted(cb func(uid uint) error) {
	c.userDeletedCallbacks = append(c.userDeletedCallbacks, cb)
}

// OnUserAdded is called on user creation.
func (c *UserChangeNotifier) OnUserAdded(cb func(uid uint) error) {
	c.userAddedCallbacks = append(c.userAddedCallbacks, cb)
}

func (c *UserChangeNotifier) fireUserDeleted(uid uint) error {
	for _, cb := range c.userDeletedCallbacks {
		if err := cb(uid); err != nil {
			return err
		}
	}
	return nil
}

func (c *UserChangeNotifier) fireUserAdded(uid uint) error {
	for _, cb := range c.userAddedCallbacks {
		if err := cb(uid); err != nil {
			return err
		}
	}
	return nil
}

// The UserAPI provides handlers for managing users.
type UserAPI struct {
	DB                 UserDatabase
	PasswordStrength   int
	UserChangeNotifier *UserChangeNotifier
	EnableRegistration bool
}

// GetUsers returns all the users
// swagger:operation GET /user user getUsers
//
// Return all users.
//
// ---
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// responses:
//   200:
//     description: Ok
//     schema:
//       type: array
//       items:
//         $ref: "#/definitions/User"
//   401:
//     description: Unauthorized
//     schema:
//         $ref: "#/definitions/Error"
//   403:
//     description: Forbidden
//     schema:
//         $ref: "#/definitions/Error"
func (a *UserAPI) GetUsers(ctx *gin.Context) {
	users, err := a.DB.GetUsers()
	if success := successOrAbort(ctx, 500, err); !success {
		return
	}
	var resp []*model.UserExternal
	for _, user := range users {
		resp = append(resp, toExternalUser(user))
	}

	ctx.JSON(200, resp)
}

// GetCurrentUser returns the current user
// swagger:operation GET /current/user user currentUser
//
// Return the current user.
//
// ---
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// responses:
//   200:
//     description: Ok
//     schema:
//         $ref: "#/definitions/User"
//   401:
//     description: Unauthorized
//     schema:
//         $ref: "#/definitions/Error"
//   403:
//     description: Forbidden
//     schema:
//         $ref: "#/definitions/Error"
func (a *UserAPI) GetCurrentUser(ctx *gin.Context) {
	user, err := a.DB.GetUserByID(auth.GetUserID(ctx))
	if success := successOrAbort(ctx, 500, err); !success {
		return
	}
	ctx.JSON(200, toExternalUser(user))
}

// CreateUser creates a user.
// Granting admin to the new user depends on the admin state of the current user.
// Creating a new user by a non-admin user also checks the registration configuration parameter.
// swagger:operation POST /user user createUser
//
// Create a user.
//
// ---
// consumes: [application/json]
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// parameters:
// - name: body
//   in: body
//   description: the user to add
//   required: true
//   schema:
//     $ref: "#/definitions/UserWithPass"
// responses:
//   200:
//     description: Ok
//     schema:
//         $ref: "#/definitions/User"
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
func (a *UserAPI) CreateUser(ctx *gin.Context) {
	user := model.UserExternalWithPass{}
	if err := ctx.Bind(&user); err == nil {
		internal := a.toInternalUser(&user, []byte{})
		existingUser, err := a.DB.GetUserByName(internal.Name)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}

		requestByAdmin := false
		uid := auth.TryGetUserID(ctx)
		if uid != nil {
			requestByUser, err := a.DB.GetUserByID(*uid)
			if err == nil && requestByUser != nil {
				requestByAdmin = requestByUser.Admin // logged in as valid user
			}
		}

		if !requestByAdmin { // if the request is not from an admin
			if !a.EnableRegistration { // if not admin and no anonymous registration
				ctx.AbortWithError(http.StatusForbidden, errors.New("you are not allowed to access this api"))
				return
			}
			if internal.Admin { // not admin and wants to create a new user with admin
				ctx.AbortWithError(http.StatusUnauthorized, errors.New("you are not permitted to register an admin user"))
				return
			}
		}

		if existingUser == nil {
			if success := successOrAbort(ctx, 500, a.DB.CreateUser(internal)); !success {
				return
			}
			if err := a.UserChangeNotifier.fireUserAdded(internal.ID); err != nil {
				ctx.AbortWithError(500, err)
				return
			}
			ctx.JSON(200, toExternalUser(internal))
		} else {
			ctx.AbortWithError(400, errors.New("username already exists"))
		}
	}
}

// GetUserByID returns the user by id
// swagger:operation GET /user/{id} user getUser
//
// Get a user.
//
// ---
// consumes: [application/json]
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// parameters:
// - name: id
//   in: path
//   description: the user id
//   required: true
//   type: integer
//   format: int64
// responses:
//   200:
//     description: Ok
//     schema:
//         $ref: "#/definitions/User"
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
func (a *UserAPI) GetUserByID(ctx *gin.Context) {
	withID(ctx, "id", func(id uint) {
		user, err := a.DB.GetUserByID(id)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		if user != nil {
			ctx.JSON(200, toExternalUser(user))
		} else {
			ctx.AbortWithError(404, errors.New("user does not exist"))
		}
	})
}

// DeleteUserByID deletes the user by id
// swagger:operation DELETE /user/{id} user deleteUser
//
// Deletes a user.
//
// ---
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// parameters:
// - name: id
//   in: path
//   description: the user id
//   required: true
//   type: integer
//   format: int64
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
func (a *UserAPI) DeleteUserByID(ctx *gin.Context) {
	withID(ctx, "id", func(id uint) {
		user, err := a.DB.GetUserByID(id)
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		if user != nil {
			adminCount, err := a.DB.CountUser(&model.User{Admin: true})
			if success := successOrAbort(ctx, 500, err); !success {
				return
			}
			if user.Admin && adminCount == 1 {
				ctx.AbortWithError(400, errors.New("cannot delete last admin"))
				return
			}
			if err := a.UserChangeNotifier.fireUserDeleted(id); err != nil {
				ctx.AbortWithError(500, err)
				return
			}
			successOrAbort(ctx, 500, a.DB.DeleteUserByID(id))
		} else {
			ctx.AbortWithError(404, errors.New("user does not exist"))
		}
	})
}

// ChangePassword changes the password from the current user
// swagger:operation POST /current/user/password user updateCurrentUser
//
// Update the password of the current user.
//
// ---
// consumes: [application/json]
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// parameters:
// - name: body
//   in: body
//   description: the user
//   required: true
//   schema:
//     $ref: "#/definitions/UserPass"
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
func (a *UserAPI) ChangePassword(ctx *gin.Context) {
	pw := model.UserExternalPass{}
	if err := ctx.Bind(&pw); err == nil {
		user, err := a.DB.GetUserByID(auth.GetUserID(ctx))
		if success := successOrAbort(ctx, 500, err); !success {
			return
		}
		user.Pass = password.CreatePassword(pw.Pass, a.PasswordStrength)
		successOrAbort(ctx, 500, a.DB.UpdateUser(user))
	}
}

// UpdateUserByID updates and user by id
// swagger:operation POST /user/{id} user updateUser
//
// Update a user.
//
// ---
// consumes: [application/json]
// produces: [application/json]
// security: [clientTokenHeader: [], clientTokenQuery: [], basicAuth: []]
// parameters:
// - name: id
//   in: path
//   description: the user id
//   required: true
//   type: integer
//   format: int64
// - name: body
//   in: body
//   description: the updated user
//   required: true
//   schema:
//     $ref: "#/definitions/UserWithPass"
// responses:
//   200:
//     description: Ok
//     schema:
//         $ref: "#/definitions/User"
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
func (a *UserAPI) UpdateUserByID(ctx *gin.Context) {
	withID(ctx, "id", func(id uint) {
		var user *model.UserExternalWithPass
		if err := ctx.Bind(&user); err == nil {
			oldUser, err := a.DB.GetUserByID(id)
			if success := successOrAbort(ctx, 500, err); !success {
				return
			}
			if oldUser != nil {
				adminCount, err := a.DB.CountUser(&model.User{Admin: true})
				if success := successOrAbort(ctx, 500, err); !success {
					return
				}
				if !user.Admin && oldUser.Admin && adminCount == 1 {
					ctx.AbortWithError(400, errors.New("cannot delete last admin"))
					return
				}
				internal := a.toInternalUser(user, oldUser.Pass)
				internal.ID = id
				if success := successOrAbort(ctx, 500, a.DB.UpdateUser(internal)); !success {
					return
				}
				ctx.JSON(200, toExternalUser(internal))
			} else {
				ctx.AbortWithError(404, errors.New("user does not exist"))
			}
		}
	})
}

func (a *UserAPI) toInternalUser(response *model.UserExternalWithPass, pw []byte) *model.User {
	user := &model.User{
		Name:  response.Name,
		Admin: response.Admin,
	}
	if response.Pass != "" {
		user.Pass = password.CreatePassword(response.Pass, a.PasswordStrength)
	} else {
		user.Pass = pw
	}
	return user
}

func toExternalUser(internal *model.User) *model.UserExternal {
	return &model.UserExternal{
		Name:  internal.Name,
		Admin: internal.Admin,
		ID:    internal.ID,
	}
}
