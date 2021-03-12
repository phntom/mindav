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
	requestUri := c.Request.RequestURI
	newUri := fmt.Sprintf("/v1/webdav/u/%s/%s", authUser, requestUri[11:])
	c.Request.RequestURI = newUri
	c.Request.URL.Path = newUri

	requestUri = c.Request.Header.Get("Destination")
	if len(requestUri) > 0 {
		uriPrefixLen := len(c.Request.Header.Get("X-URI-Prefix"))
		newUri = fmt.Sprintf("http://%s/v1/webdav/u/%s/%s", c.Request.Host, authUser, requestUri[uriPrefixLen:])
		c.Request.Header.Set("Destination", newUri)
	}

	mindav.Handler().ServeHTTP(c.Writer, c.Request)
	return
}
