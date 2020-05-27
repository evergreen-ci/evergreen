package aviation

import (
	"context"
	"fmt"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func MakeGripUnaryInterceptor(logger grip.Journaler) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		startAt := time.Now()
		id := getNumber()
		ctx = SetRequestID(ctx, id)
		ctx = SetRequestStartAt(ctx, startAt)

		defer func() {
			err = recovery.SendMessageWithPanicError(recover(), err, logger, message.Fields{
				"request":     id,
				"duration_ms": int64(time.Since(startAt) / time.Millisecond),
				"method":      info.FullMethod,
				"type":        "unary",
				"action":      "aborted",
			})
		}()

		logger.Debug(message.Fields{
			"action":  "started",
			"request": id,
			"method":  info.FullMethod,
			"type":    "unary",
		})

		resp, err = handler(ctx, req)
		stat := status.Convert(err)

		m := message.Fields{
			"action":        "complete",
			"type":          "unary",
			"request":       id,
			"request_type":  fmt.Sprintf("%T", req),
			"response_type": fmt.Sprintf("%T", resp),
			"service_type":  fmt.Sprintf("%T", info.Server),
			"duration_ms":   int64(time.Since(startAt) / time.Millisecond),
			"has_error":     err != nil,
			"error_msg":     err,
			"method":        info.FullMethod,
			"code":          stat.Code().String(),
		}

		if desc := stat.Message(); desc != "" {
			m["desc"] = desc
		}

		if m["has_error"] == true {
			logger.Error(m)
		} else {
			logger.Debug(m)
		}

		return
	}
}

func MakeGripStreamInterceptor(logger grip.Journaler) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		startAt := time.Now()
		id := getNumber()

		defer func() {
			err = recovery.SendMessageWithPanicError(recover(), err, logger, message.Fields{
				"request":     id,
				"duration_ms": int64(time.Since(startAt) / time.Millisecond),
				"method":      info.FullMethod,
				"type":        "stream",
				"action":      "aborted",
			})
		}()

		logger.Debug(message.Fields{
			"action":  "started",
			"request": id,
			"method":  info.FullMethod,
			"type":    "stream",
		})

		err = handler(srv, stream)
		stat := status.Convert(err)

		m := message.Fields{
			"action":       "complete",
			"type":         "stream",
			"request":      id,
			"service_type": fmt.Sprintf("%T", srv),
			"duration_ms":  int64(time.Since(startAt) / time.Millisecond),
			"has_error":    err != nil,
			"error_msg":    err,
			"method":       info.FullMethod,
			"code":         stat.Code().String(),
		}

		if desc := stat.Message(); desc != "" {
			m["desc"] = desc
		}

		if m["has_error"] == true {
			logger.Error(m)
		} else {
			logger.Debug(m)
		}

		return
	}
}
