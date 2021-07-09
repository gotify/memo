package api

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gotify/server/v2/mode"
	"github.com/gotify/server/v2/model"
	"github.com/gotify/server/v2/test"
	"github.com/gotify/server/v2/test/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

var (
	firstApplicationToken  = "Aaaaaaaaaaaaaaa"
	secondApplicationToken = "Abbbbbbbbbbbbbb"
)

func TestApplicationSuite(t *testing.T) {
	suite.Run(t, new(ApplicationSuite))
}

type ApplicationSuite struct {
	suite.Suite
	db       *testdb.Database
	a        *ApplicationAPI
	ctx      *gin.Context
	recorder *httptest.ResponseRecorder
}

var (
	originalGenerateApplicationToken func() string
	originalGenerateImageName        func() string
)

func (s *ApplicationSuite) BeforeTest(suiteName, testName string) {
	originalGenerateApplicationToken = generateApplicationToken
	originalGenerateImageName = generateImageName
	generateApplicationToken = test.Tokens(firstApplicationToken, secondApplicationToken)
	generateImageName = test.Tokens(firstApplicationToken[1:], secondApplicationToken[1:])
	mode.Set(mode.TestDev)
	s.recorder = httptest.NewRecorder()
	s.db = testdb.NewDB(s.T())
	s.ctx, _ = gin.CreateTestContext(s.recorder)
	withURL(s.ctx, "http", "example.com")
	s.a = &ApplicationAPI{DB: s.db}
}

func (s *ApplicationSuite) AfterTest(suiteName, testName string) {
	generateApplicationToken = originalGenerateApplicationToken
	generateImageName = originalGenerateImageName
	s.db.Close()
}

func (s *ApplicationSuite) Test_CreateApplication_mapAllParameters() {
	s.db.User(5)

	test.WithUser(s.ctx, 5)
	s.withFormData("name=custom_name&description=description_text")
	s.a.CreateApplication(s.ctx)

	expected := &model.Application{
		ID:          1,
		Token:       firstApplicationToken,
		UserID:      5,
		Name:        "custom_name",
		Description: "description_text",
	}
	assert.Equal(s.T(), 200, s.recorder.Code)
	if app, err := s.db.GetApplicationByID(1); assert.NoError(s.T(), err) {
		assert.Equal(s.T(), expected, app)
	}
}

func (s *ApplicationSuite) Test_ensureApplicationHasCorrectJsonRepresentation() {
	actual := &model.Application{
		ID:          1,
		UserID:      2,
		Token:       "Aasdasfgeeg",
		Name:        "myapp",
		Description: "mydesc",
		Image:       "asd",
		Internal:    true,
	}
	test.JSONEquals(s.T(), actual, `{"id":1,"token":"Aasdasfgeeg","name":"myapp","description":"mydesc", "image": "asd", "internal":true}`)
}

func (s *ApplicationSuite) Test_CreateApplication_expectBadRequestOnEmptyName() {
	s.db.User(5)

	test.WithUser(s.ctx, 5)
	s.withFormData("name=&description=description_text")
	s.a.CreateApplication(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
	if app, err := s.db.GetApplicationsByUser(5); assert.NoError(s.T(), err) {
		assert.Empty(s.T(), app)
	}
}

func (s *ApplicationSuite) Test_DeleteApplication_expectNotFoundOnCurrentUserIsNotOwner() {
	s.db.User(2)
	s.db.User(5).App(5)

	test.WithUser(s.ctx, 2)
	s.ctx.Request = httptest.NewRequest("DELETE", "/token/5", nil)
	s.ctx.Params = gin.Params{{Key: "id", Value: "5"}}

	s.a.DeleteApplication(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
	s.db.AssertAppExist(5)
}

func (s *ApplicationSuite) Test_CreateApplication_onlyRequiredParameters() {
	s.db.User(5)

	test.WithUser(s.ctx, 5)
	s.withFormData("name=custom_name")
	s.a.CreateApplication(s.ctx)

	expected := &model.Application{ID: 1, Token: firstApplicationToken, Name: "custom_name", UserID: 5}
	assert.Equal(s.T(), 200, s.recorder.Code)
	if app, err := s.db.GetApplicationsByUser(5); assert.NoError(s.T(), err) {
		assert.Contains(s.T(), app, expected)
	}
}

func (s *ApplicationSuite) Test_CreateApplication_returnsApplicationWithID() {
	s.db.User(5)

	test.WithUser(s.ctx, 5)
	s.withFormData("name=custom_name")

	s.a.CreateApplication(s.ctx)

	expected := &model.Application{
		ID:     1,
		Token:  firstApplicationToken,
		Name:   "custom_name",
		Image:  "static/defaultapp.png",
		UserID: 5,
	}
	assert.Equal(s.T(), 200, s.recorder.Code)
	test.BodyEquals(s.T(), expected, s.recorder)
}

func (s *ApplicationSuite) Test_CreateApplication_withExistingToken() {
	s.db.User(5)
	s.db.User(6).AppWithToken(1, firstApplicationToken)

	test.WithUser(s.ctx, 5)
	s.withFormData("name=custom_name")

	s.a.CreateApplication(s.ctx)

	expected := &model.Application{ID: 2, Token: secondApplicationToken, Name: "custom_name", UserID: 5}
	assert.Equal(s.T(), 200, s.recorder.Code)
	if app, err := s.db.GetApplicationsByUser(5); assert.NoError(s.T(), err) {
		assert.Contains(s.T(), app, expected)
	}
}

func (s *ApplicationSuite) Test_GetApplications() {
	userBuilder := s.db.User(5)
	first := userBuilder.NewAppWithToken(1, "perfper")
	second := userBuilder.NewAppWithToken(2, "asdasd")

	test.WithUser(s.ctx, 5)
	s.ctx.Request = httptest.NewRequest("GET", "/tokens", nil)

	s.a.GetApplications(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	first.Image = "static/defaultapp.png"
	second.Image = "static/defaultapp.png"
	test.BodyEquals(s.T(), []*model.Application{first, second}, s.recorder)
}

func (s *ApplicationSuite) Test_GetApplications_WithImage() {
	userBuilder := s.db.User(5)
	first := userBuilder.NewAppWithToken(1, "perfper")
	second := userBuilder.NewAppWithToken(2, "asdasd")
	first.Image = "abcd.jpg"
	s.db.UpdateApplication(first)

	test.WithUser(s.ctx, 5)
	s.ctx.Request = httptest.NewRequest("GET", "/tokens", nil)

	s.a.GetApplications(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	first.Image = "image/abcd.jpg"
	second.Image = "static/defaultapp.png"
	test.BodyEquals(s.T(), []*model.Application{first, second}, s.recorder)
}

func (s *ApplicationSuite) Test_DeleteApplication_internal_expectBadRequest() {
	s.db.User(5).InternalApp(10)

	test.WithUser(s.ctx, 5)
	s.ctx.Request = httptest.NewRequest("DELETE", "/token/"+firstApplicationToken, nil)
	s.ctx.Params = gin.Params{{Key: "id", Value: "10"}}

	s.a.DeleteApplication(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *ApplicationSuite) Test_DeleteApplication_expectNotFound() {
	s.db.User(5)

	test.WithUser(s.ctx, 5)
	s.ctx.Request = httptest.NewRequest("DELETE", "/token/"+firstApplicationToken, nil)
	s.ctx.Params = gin.Params{{Key: "id", Value: "4"}}

	s.a.DeleteApplication(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *ApplicationSuite) Test_DeleteApplication() {
	s.db.User(5).App(1)

	test.WithUser(s.ctx, 5)
	s.ctx.Request = httptest.NewRequest("DELETE", "/token/"+firstApplicationToken, nil)
	s.ctx.Params = gin.Params{{Key: "id", Value: "1"}}

	s.a.DeleteApplication(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	s.db.AssertAppNotExist(1)
}

func (s *ApplicationSuite) Test_UploadAppImage_NoImageProvided_expectBadRequest() {
	s.db.User(5).App(1)
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	writer.Close()
	s.ctx.Request = httptest.NewRequest("POST", "/irrelevant", &b)
	s.ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "1"}}

	s.a.UploadApplicationImage(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
	assert.Equal(s.T(), s.ctx.Errors[0].Err, errors.New("file with key 'file' must be present"))
}

func (s *ApplicationSuite) Test_UploadAppImage_OtherErrors_expectServerError() {
	s.db.User(5).App(1)
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	defer writer.Close()
	s.ctx.Request = httptest.NewRequest("POST", "/irrelevant", &b)
	s.ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "1"}}

	s.a.UploadApplicationImage(s.ctx)

	assert.Equal(s.T(), 500, s.recorder.Code)
	assert.Equal(s.T(), s.ctx.Errors[0].Err, errors.New("multipart: NextPart: EOF"))
}

func (s *ApplicationSuite) Test_UploadAppImage_WithImageFile_expectSuccess() {
	s.db.User(5).App(1)

	cType, buffer, err := upload(map[string]*os.File{"file": mustOpen("../test/assets/image.png")})
	assert.Nil(s.T(), err)
	s.ctx.Request = httptest.NewRequest("POST", "/irrelevant", &buffer)
	s.ctx.Request.Header.Set("Content-Type", cType)
	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "1"}}

	s.a.UploadApplicationImage(s.ctx)

	if app, err := s.db.GetApplicationByID(1); assert.NoError(s.T(), err) {
		imgName := app.Image

		assert.Equal(s.T(), 200, s.recorder.Code)
		_, err = os.Stat(imgName)
		assert.Nil(s.T(), err)

		s.a.DeleteApplication(s.ctx)

		_, err = os.Stat(imgName)
		assert.True(s.T(), os.IsNotExist(err))
	}
}

func (s *ApplicationSuite) Test_UploadAppImage_WithImageFile_DeleteExstingImageAndGenerateNewName() {
	existingImageName := "2lHMAel6BDHLL-HrwphcviX-l.png"
	firstGeneratedImageName := firstApplicationToken[1:] + ".png"
	secondGeneratedImageName := secondApplicationToken[1:] + ".png"
	s.db.User(5)
	s.db.CreateApplication(&model.Application{UserID: 5, ID: 1, Image: existingImageName})

	cType, buffer, err := upload(map[string]*os.File{"file": mustOpen("../test/assets/image.png")})
	assert.Nil(s.T(), err)
	s.ctx.Request = httptest.NewRequest("POST", "/irrelevant", &buffer)
	s.ctx.Request.Header.Set("Content-Type", cType)
	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	fakeImage(s.T(), existingImageName)
	fakeImage(s.T(), firstGeneratedImageName)

	s.a.UploadApplicationImage(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)

	_, err = os.Stat(existingImageName)
	assert.True(s.T(), os.IsNotExist(err))

	_, err = os.Stat(secondGeneratedImageName)
	assert.Nil(s.T(), err)
	assert.Nil(s.T(), os.Remove(secondGeneratedImageName))
	assert.Nil(s.T(), os.Remove(firstGeneratedImageName))
}

func (s *ApplicationSuite) Test_UploadAppImage_WithImageFile_DeleteExistingImage() {
	s.db.User(5)
	s.db.CreateApplication(&model.Application{UserID: 5, ID: 1, Image: "existing.png"})

	fakeImage(s.T(), "existing.png")
	cType, buffer, err := upload(map[string]*os.File{"file": mustOpen("../test/assets/image.png")})
	assert.Nil(s.T(), err)
	s.ctx.Request = httptest.NewRequest("POST", "/irrelevant", &buffer)
	s.ctx.Request.Header.Set("Content-Type", cType)
	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "1"}}

	s.a.UploadApplicationImage(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)

	_, err = os.Stat("existing.png")
	assert.True(s.T(), os.IsNotExist(err))

	os.Remove(firstApplicationToken[1:] + ".png")
}

func (s *ApplicationSuite) Test_UploadAppImage_WithTextFile_expectBadRequest() {
	s.db.User(5).App(1)

	cType, buffer, err := upload(map[string]*os.File{"file": mustOpen("../test/assets/text.txt")})
	assert.Nil(s.T(), err)
	s.ctx.Request = httptest.NewRequest("POST", "/irrelevant", &buffer)
	s.ctx.Request.Header.Set("Content-Type", cType)
	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "1"}}

	s.a.UploadApplicationImage(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
	assert.Equal(s.T(), s.ctx.Errors[0].Err, errors.New("file must be an image"))
}

func (s *ApplicationSuite) Test_UploadAppImage_expectNotFound() {
	s.db.User(5)

	test.WithUser(s.ctx, 5)
	s.ctx.Request = httptest.NewRequest("POST", "/irrelevant", nil)
	s.ctx.Params = gin.Params{{Key: "id", Value: "4"}}

	s.a.UploadApplicationImage(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *ApplicationSuite) Test_UploadAppImage_WithSaveError_expectServerError() {
	s.db.User(5).App(1)

	cType, buffer, err := upload(map[string]*os.File{"file": mustOpen("../test/assets/image.png")})
	assert.Nil(s.T(), err)
	s.ctx.Request = httptest.NewRequest("POST", "/irrelevant/", &buffer)
	s.a.ImageDir = "asdasd/asdasda/asdasd"
	s.ctx.Request.Header.Set("Content-Type", cType)
	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "1"}}

	s.a.UploadApplicationImage(s.ctx)

	assert.Equal(s.T(), 500, s.recorder.Code)
}

func (s *ApplicationSuite) Test_UpdateApplicationNameAndDescription_expectSuccess() {
	s.db.User(5).NewAppWithToken(2, "app-2")

	test.WithUser(s.ctx, 5)
	s.withFormData("name=new_name&description=new_description_text")
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}
	s.a.UpdateApplication(s.ctx)

	expected := &model.Application{
		ID:          2,
		Token:       "app-2",
		UserID:      5,
		Name:        "new_name",
		Description: "new_description_text",
	}

	assert.Equal(s.T(), 200, s.recorder.Code)
	if app, err := s.db.GetApplicationByID(2); assert.NoError(s.T(), err) {
		assert.Equal(s.T(), expected, app)
	}
}

func (s *ApplicationSuite) Test_UpdateApplicationName_expectSuccess() {
	s.db.User(5).NewAppWithToken(2, "app-2")

	test.WithUser(s.ctx, 5)
	s.withFormData("name=new_name")
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}
	s.a.UpdateApplication(s.ctx)

	expected := &model.Application{
		ID:          2,
		Token:       "app-2",
		UserID:      5,
		Name:        "new_name",
		Description: "",
	}

	assert.Equal(s.T(), 200, s.recorder.Code)
	if app, err := s.db.GetApplicationByID(2); assert.NoError(s.T(), err) {
		assert.Equal(s.T(), expected, app)
	}
}

func (s *ApplicationSuite) Test_UpdateApplication_preservesImage() {
	app := s.db.User(5).NewAppWithToken(2, "app-2")
	app.Image = "existing.png"
	assert.Nil(s.T(), s.db.UpdateApplication(app))

	test.WithUser(s.ctx, 5)
	s.withFormData("name=new_name")
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	s.a.UpdateApplication(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	if app, err := s.db.GetApplicationByID(2); assert.NoError(s.T(), err) {
		assert.Equal(s.T(), "existing.png", app.Image)
	}
}

func (s *ApplicationSuite) Test_UpdateApplication_setEmptyDescription() {
	app := s.db.User(5).NewAppWithToken(2, "app-2")
	app.Description = "my desc"
	assert.Nil(s.T(), s.db.UpdateApplication(app))

	test.WithUser(s.ctx, 5)
	s.withFormData("name=new_name&desc=")
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	s.a.UpdateApplication(s.ctx)

	assert.Equal(s.T(), 200, s.recorder.Code)
	if app, err := s.db.GetApplicationByID(2); assert.NoError(s.T(), err) {
		assert.Equal(s.T(), "", app.Description)
	}
}

func (s *ApplicationSuite) Test_UpdateApplication_expectNotFound() {
	test.WithUser(s.ctx, 5)
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}
	s.a.UpdateApplication(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *ApplicationSuite) Test_UpdateApplication_WithMissingAttributes_expectBadRequest() {
	test.WithUser(s.ctx, 5)
	s.a.UpdateApplication(s.ctx)

	assert.Equal(s.T(), 400, s.recorder.Code)
}

func (s *ApplicationSuite) Test_UpdateApplication_WithoutPermission_expectNotFound() {
	s.db.User(5).NewAppWithToken(2, "app-2")

	test.WithUser(s.ctx, 4)
	s.ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	s.a.UpdateApplication(s.ctx)

	assert.Equal(s.T(), 404, s.recorder.Code)
}

func (s *ApplicationSuite) withFormData(formData string) {
	s.ctx.Request = httptest.NewRequest("POST", "/token", strings.NewReader(formData))
	s.ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
}

// A modified version of https://stackoverflow.com/a/20397167/4244993 from Attila O.
func upload(values map[string]*os.File) (contentType string, buffer bytes.Buffer, err error) {
	w := multipart.NewWriter(&buffer)
	for key, r := range values {
		var fw io.Writer
		if fw, err = w.CreateFormFile(key, r.Name()); err != nil {
			return
		}

		if _, err = io.Copy(fw, r); err != nil {
			return
		}
	}
	contentType = w.FormDataContentType()
	w.Close()
	return
}

func mustOpen(f string) *os.File {
	r, err := os.Open(f)
	if err != nil {
		panic(err)
	}
	return r
}

func fakeImage(t *testing.T, path string) {
	data, err := ioutil.ReadFile("../test/assets/image.png")
	assert.Nil(t, err)
	// Write data to dst
	err = ioutil.WriteFile(path, data, 0o644)
	assert.Nil(t, err)
}
