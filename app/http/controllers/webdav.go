package controllers

import (
	"fmt"
	"github.com/totoval/framework/http/controller"
	"github.com/totoval/framework/request"
	"totoval/app/logics/mindav"
)

type WebDAV struct {
	controller.BaseController
}

func (wd *WebDAV) Handle(c *request.Context) {
	authUser := c.Request.Header.Get("X-Auth-Request-User")
	requestUri := c.Request.RequestURI[11:]
	newUri := fmt.Sprintf("/v1/webdav/u/%s/%s", authUser, requestUri)
	c.Request.RequestURI = newUri
	c.Request.URL.Path = newUri
	mindav.Handler().ServeHTTP(c.Writer, c.Request)
	return
}
