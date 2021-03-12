package main

import (
	"context"
	"encoding/base64"
	"github.com/gin-gonic/gin"
	"github.com/mattermost/mattermost-server/v5/model"
	c "github.com/totoval/framework/config"
	"github.com/totoval/framework/graceful"
	"github.com/totoval/framework/helpers/log"
	"github.com/totoval/framework/helpers/toto"
	"github.com/totoval/framework/helpers/zone"
	"github.com/totoval/framework/http/middleware"
	"github.com/totoval/framework/request"
	"github.com/totoval/framework/sentry"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"totoval/bootstrap"
	"totoval/resources/views"
	"totoval/routes"
)

func init() {
	bootstrap.Initialize()
}

// @caution cannot use config methods to get config in init function
func main() {
	//j := &jobs.ExampleJob{}
	//j.SetParam(&pbs.ExampleJob{Query: "test", PageNumber: 111, ResultPerPage: 222})
	////j.SetDelay(5 * zone.Second)
	//err := job.Dispatch(j)
	//fmt.Println(err)

	//go hub.On("add-user-affiliation")  // go run artisan.go queue:listen add-user-affiliation

	ctx, cancel := context.WithCancel(context.Background())

	quit := make(chan os.Signal, 1)
	// kill (no param) default send syscanll.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall. SIGKILL but can"t be catch, so don't need add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		call := <-quit
		log.Info("system call", toto.V{"call": call})
		cancel()
	}()

	httpServe(ctx)
}

func httpServe(ctx context.Context) {
	r := request.New()

	sentry.Use(r.GinEngine(), false)

	if c.GetBool("app.debug") {
		//r.Use(middleware.RequestLogger())
	}

	r.RedirectTrailingSlash = false

	if c.GetString("app.env") == "production" {
		r.Use(middleware.Logger())
		r.Use(middleware.Recovery())
	}

	r.Use(middleware.Locale())

	r.UseGin(func(c *gin.Context) {
		//t := time.Now()

		// Set example variable
		authUser := c.Request.Header.Get("X-Auth-Request-User")

		if len(authUser) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"Error": "Access Denied"})
			c.Abort()
			return
		}

		if authUser == "kixtoken@" {
			auth := strings.SplitN(c.Request.Header.Get("Authorization"), " ", 2)
			if len(auth) != 2 || auth[0] != "Basic" {
				c.JSON(http.StatusForbidden, gin.H{"Error": "Please use basic auth with token in the password field"})
				c.Abort()
				return
			}
			payload, _ := base64.StdEncoding.DecodeString(auth[1])
			token := strings.SplitN(string(payload), ":", 2)[1]
			if !regexp.MustCompile(`^[a-z0-9]{26}$`).MatchString(token) {
				c.JSON(http.StatusForbidden, gin.H{"Error": "Invalid personal access token, please generate at https://kix.co.il Account Settings > Security"})
				c.Abort()
				return
			}
			mm := model.NewAPIv4Client("https://kix.co.il")
			mm.SetToken(token)
			me, response := mm.GetMe("")
			if response.StatusCode != 200 {
				c.JSON(http.StatusForbidden, gin.H{"Error": "Unauthorized personal access token, please generate at https://kix.co.il Account Settings > Security"})
				c.Abort()
				return
			}
			c.Request.Header.Set("X-Auth-Request-User", me.Email)
		}

		// before request

		c.Next()

		// after request
		//latency := time.Since(t)

		// access the status we are sending
		//status := c.Writer.Status()
	})

	routes.Register(r)

	views.Initialize(r)

	s := &http.Server{
		Addr:           ":" + c.GetString("app.port"),
		Handler:        r,
		ReadTimeout:    zone.Duration(c.GetInt64("app.read_timeout_seconds")) * zone.Second,
		WriteTimeout:   zone.Duration(c.GetInt64("app.write_timeout_seconds")) * zone.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err.Error())
		}
	}()

	<-ctx.Done()

	log.Info("Shutdown Server ...")

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	_ctx, cancel := context.WithTimeout(ctx, 5*zone.Second)
	defer cancel()

	if err := s.Shutdown(_ctx); err != nil {
		log.Fatal("Server Shutdown: ", toto.V{"error": err})
	}

	// totoval framework shutdown
	graceful.ShutDown(false)

	log.Info("Server exiting")
}
