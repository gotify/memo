package api

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gotify/server/v2/auth"
	"github.com/gotify/server/v2/auth/password"
	"github.com/gotify/server/v2/mode"
	"github.com/gotify/server/v2/model"
	"github.com/gotify/server/v2/test"
	"github.com/gotify/server/v2/test/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestUserSuite(t *testing.T) {
	suite.Run(t, new(UserSuite))
}

type UserSuite struct {
	suite.Suite
	db             *testdb.Database
	a              *UserAPI
	ctx            *gin.Context
	recorder       *httptest.ResponseRecorder
	notifiedAdd    bool
	notifiedDelete bool
	notifier       *UserChangeNotifier
}

func (s *UserSuite) BeforeTest(suiteName, testName string) {
	mode.Set(mode.TestDev)
	s.recorder = httptest.NewRecorder()
	s.ctx, _ = gin.CreateTestContext(s.recorder)

	s.db = testdb.NewDB(s.T())

	s.notifier = new(UserChangeNotifier)
	s.notifier.OnUserDeleted(func(uint) error {
		s.notifiedDelete = true
		return nil
	})
	s.notifier.OnUserAdded(func(uint) error {
		s.notifiedAdd = true
		return nil
	})
	s.a = &UserAPI{DB: s.db, UserChangeNotifier: s.notifier}

	if strings.Contains(testName, "CreateUser") { // create the users that will register for the createuser test
		s.db.CreateUser(&model.User{ID: 1, Name: "admin", Admin: true})
		s.db.CreateUser(&model.User{ID: 2, Name: "user", Admin: false})

		auth.RegisterAuthentication(s.ctx, nil, 1, "") // will be admin by default unless testing otherwise
	}
}

func (s *UserSuite) AfterTest(suiteName, testName string) {
	s.db.Close()
}

func (s *UserSuite) Test_GetUsers() {
	first := s.db.NewUser(2)
	second := s.db.NewUser(5)

	s.a.GetUsers(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	test.BodyEquals(s.T(), []*model.UserExternal{externalOf(first), externalOf(second)}, s.recorder)
}

func (s *UserSuite) Test_GetCurrentUser() {
	user := s.db.NewUser(5)

	test.WithUser(s.ctx, 5)
	s.a.GetCurrentUser(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	test.BodyEquals(s.T(), externalOf(user), s.recorder)
}

func (s *UserSuite) Test_GetUserByID() {
	user := s.db.NewUser(2)

	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	s.a.GetUserByID(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	test.BodyEquals(s.T(), externalOf(user), s.recorder)
}

func (s *UserSuite) Test_GetUserByID_InvalidID() {
	s.db.User(2)
	s.ctx.Params = gin.Params{{Key: "id", Value: "abc"}}

	s.a.GetUserByID(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *UserSuite) Test_GetUserByID_UnknownUser() {
	s.db.User(2)

	s.ctx.Params = gin.Params{{Key: "id", Value: "3"}}

	s.a.GetUserByID(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *UserSuite) Test_DeleteUserByID_LastAdmin_Expect400() {
	s.db.CreateUser(&model.User{
		ID:    7,
		Name:  "admin",
		Admin: true,
	})
	s.ctx.Params = gin.Params{{Key: "id", Value: "7"}}

	s.a.DeleteUserByID(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *UserSuite) Test_DeleteUserByID_InvalidID() {
	s.ctx.Params = gin.Params{{Key: "id", Value: "abc"}}

	s.a.DeleteUserByID(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *UserSuite) Test_DeleteUserByID_UnknownUser() {
	s.db.User(2)

	s.ctx.Params = gin.Params{{Key: "id", Value: "3"}}

	s.a.DeleteUserByID(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *UserSuite) Test_DeleteUserByID() {
	assert.False(s.T(), s.notifiedDelete)

	s.db.User(2)

	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	s.a.DeleteUserByID(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	s.db.AssertUserNotExist(2)
	assert.True(s.T(), s.notifiedDelete)
}

func (s *UserSuite) Test_DeleteUserByID_NotifyFail() {
	s.db.User(5)
	s.notifier.OnUserDeleted(func(id uint) error {
		if id == 5 {
			return errors.New("some error")
		}
		return nil
	})

	s.ctx.Params = gin.Params{{Key: "id", Value: "5"}}

	s.a.DeleteUserByID(s.ctx)

	assert.Equal(s.T(), 500, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser() {
	assert.False(s.T(), s.notifiedAdd)
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "tom", "pass": "mylittlepony", "admin": true}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	user := &model.UserExternal{ID: 3, Name: "tom", Admin: true}
	test.BodyEquals(s.T(), user, s.recorder)
	assert.Equal(s.T(), 200, s.recorder.Code)

	if created, err := s.db.GetUserByName("tom"); assert.NoError(s.T(), err) {
		assert.NotNil(s.T(), created)
		assert.True(s.T(), password.ComparePassword(created.Pass, []byte("mylittlepony")))
	}
	assert.True(s.T(), s.notifiedAdd)
}

func (s *UserSuite) Test_CreateUser_ByNonAdmin() {
	auth.RegisterAuthentication(s.ctx, nil, 2, "")
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "tom", "pass": "1", "admin": false}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 403, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser_Register_ByNonAdmin() {
	s.a.EnableRegistration = true

	auth.RegisterAuthentication(s.ctx, nil, 2, "")
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "tom", "pass": "1", "admin": false}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser_Register_Admin_ByNonAdmin() {
	s.a.EnableRegistration = true

	auth.RegisterAuthentication(s.ctx, nil, 2, "")
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "tom", "pass": "1", "admin": true}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 401, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser_Anonymous() {
	auth.RegisterAuthentication(s.ctx, nil, 0, "")
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "tom", "pass": "1", "admin": false}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 403, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser_Register_Anonymous() {
	s.a.EnableRegistration = true

	auth.RegisterAuthentication(s.ctx, nil, 0, "")
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "tom", "pass": "1", "admin": false}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser_Register_Admin_Anonymous() {
	s.a.EnableRegistration = true

	auth.RegisterAuthentication(s.ctx, nil, 0, "")
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "tom", "pass": "1", "admin": true}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 401, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser_NotifyFail() {
	s.notifier.OnUserAdded(func(id uint) error {
		user, err := s.db.GetUserByID(id)
		if err != nil {
			return err
		}
		if user.Name == "eva" {
			return errors.New("some error")
		}
		return nil
	})
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "eva", "pass": "mylittlepony", "admin": true}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 500, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser_NoPassword() {
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "tom", "pass": "", "admin": true}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser_NoName() {
	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "", "pass": "asd", "admin": true}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *UserSuite) Test_CreateUser_NameAlreadyExists() {
	s.db.NewUserWithName(3, "tom")

	s.ctx.Request = httptest.NewRequest("POST", "/user", strings.NewReader(`{"name": "tom", "pass": "mylittlepony", "admin": true}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.CreateUser(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *UserSuite) Test_UpdateUserByID_InvalidID() {
	s.ctx.Params = gin.Params{{Key: "id", Value: "abc"}}

	s.ctx.Request = httptest.NewRequest("POST", "/user/abc", strings.NewReader(`{"name": "tom", "pass": "", "admin": false}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.UpdateUserByID(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *UserSuite) Test_UpdateUserByID_LastAdmin_Expect400() {
	s.db.CreateUser(&model.User{
		ID:    7,
		Name:  "admin",
		Admin: true,
	})

	s.ctx.Params = gin.Params{{Key: "id", Value: "7"}}

	s.ctx.Request = httptest.NewRequest("POST", "/user/7", strings.NewReader(`{"name": "admin", "pass": "", "admin": false}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")
	s.a.UpdateUserByID(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *UserSuite) Test_UpdateUserByID_UnknownUser() {
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	s.ctx.Request = httptest.NewRequest("POST", "/user/2", strings.NewReader(`{"name": "tom", "pass": "", "admin": false}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.UpdateUserByID(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *UserSuite) Test_UpdateUserByID_UpdateNotPassword() {
	s.db.CreateUser(&model.User{ID: 2, Name: "nico", Pass: password.CreatePassword("old", 5)})

	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	s.ctx.Request = httptest.NewRequest("POST", "/user/2", strings.NewReader(`{"name": "tom", "pass": "", "admin": true}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.UpdateUserByID(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	user, err := s.db.GetUserByID(2)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), user)
	assert.True(s.T(), password.ComparePassword(user.Pass, []byte("old")))
}

func (s *UserSuite) Test_UpdateUserByID_UpdatePassword() {
	s.db.CreateUser(&model.User{ID: 2, Name: "tom", Pass: password.CreatePassword("old", 5)})

	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	s.ctx.Request = httptest.NewRequest("POST", "/user/2", strings.NewReader(`{"name": "tom", "pass": "new", "admin": true}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.UpdateUserByID(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	user, err := s.db.GetUserByID(2)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), user)
	assert.True(s.T(), password.ComparePassword(user.Pass, []byte("new")))
}

func (s *UserSuite) Test_UpdatePassword() {
	s.db.CreateUser(&model.User{ID: 1, Name: "jmattheis", Pass: password.CreatePassword("old", 5)})

	test.WithUser(s.ctx, 1)
	s.ctx.Request = httptest.NewRequest("POST", "/user/current/password", strings.NewReader(`{"pass": "new"}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.ChangePassword(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	user, err := s.db.GetUserByID(1)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), user)
	assert.True(s.T(), password.ComparePassword(user.Pass, []byte("new")))
}

func (s *UserSuite) Test_UpdatePassword_EmptyPassword() {
	s.db.CreateUser(&model.User{ID: 1, Name: "jmattheis", Pass: password.CreatePassword("old", 5)})

	test.WithUser(s.ctx, 1)
	s.ctx.Request = httptest.NewRequest("POST", "/user/current/password", strings.NewReader(`{"pass":""}`))
	s.ctx.Request.Header.Set("Content-Type", "application/json")

	s.a.ChangePassword(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
	user, err := s.db.GetUserByID(1)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), user)
	assert.True(s.T(), password.ComparePassword(user.Pass, []byte("old")))
}

func externalOf(user *model.User) *model.UserExternal {
	return &model.UserExternal{Name: user.Name, Admin: user.Admin, ID: user.ID}
}
