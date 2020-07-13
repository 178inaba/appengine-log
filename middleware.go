package logging

import (
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"contrib.go.opencensus.io/exporter/stackdriver/propagation"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

type contextLogger struct {
	echo.Context
	logger echo.Logger
}

func (l *contextLogger) Logger() echo.Logger {
	return l.logger
}

// LoggerMiddleware is appengine echo logger middleware.
type LoggerMiddleware struct {
	client *logging.Client

	moduleID  string
	projectID string
	versionID string
	zone      string
}

// NewLoggerMiddleware returns appengine echo logger middleware.
func NewLoggerMiddleware(client *logging.Client, moduleID, projectID, versionID, zone string) *LoggerMiddleware {
	return &LoggerMiddleware{client: client, moduleID: moduleID, projectID: projectID, versionID: versionID, zone: zone}
}

// Logger is appengine echo logger middleware.
// Set application logger to echo.Context and write request log.
func (m *LoggerMiddleware) Logger(next echo.HandlerFunc) echo.HandlerFunc {
	hf := &propagation.HTTPFormat{}

	opt := logging.CommonResource(&mrpb.MonitoredResource{
		Type: "gae_app",
		Labels: map[string]string{
			"module_id":  m.moduleID,
			"project_id": m.projectID,
			"version_id": m.versionID,
			"zone":       m.zone,
		},
	})
	reqLogger := m.client.Logger(fmt.Sprintf("%s_request", m.moduleID), opt)
	appLogger := m.client.Logger(fmt.Sprintf("%s_application", m.moduleID), opt)
	return func(c echo.Context) error {
		req := c.Request()
		sc, _ := hf.SpanContextFromRequest(req)
		trace := fmt.Sprintf("projects/%s/traces/%s", m.projectID, sc.TraceID)
		spanID := sc.SpanID.String()

		logger := New(appLogger, trace, spanID)
		logger.SetLevel(log.INFO)

		start := time.Now()
		if err := next(&contextLogger{Context: c, logger: logger}); err != nil {
			c.Error(err)
		}
		end := time.Now()

		resp := c.Response()
		remoteIP := strings.Split(req.Header.Get("X-Forwarded-For"), ",")[0]
		reqLogger.Log(logging.Entry{
			Timestamp: time.Now(),
			Severity:  logger.maxSeverity,
			HTTPRequest: &logging.HTTPRequest{
				Request:      req,
				Latency:      end.Sub(start),
				Status:       resp.Status,
				RemoteIP:     remoteIP,
				ResponseSize: resp.Size,
			},
			Trace:        trace,
			TraceSampled: true,
			SpanID:       spanID,
		})

		return nil
	}
}
