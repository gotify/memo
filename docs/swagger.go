package docs

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gobuffalo/packr"
)

// Serve serves the documentation.
func Serve(ctx *gin.Context) {
	ctx.Writer.WriteString(get(ctx.Request.URL.Host))
}

func get(host string) string {
	box := packr.NewBox("./")
	return strings.Replace(box.String("spec.json"), "localhost", host, 1)
}
