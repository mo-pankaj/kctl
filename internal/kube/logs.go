package kube

import (
	"context"
	"fmt"
	"io"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"

	"github.com/mo-pankaj/kctl/internal/core"
)

const defaultTailLines = 500

// Logs opens a log stream for one pod container. The caller must Close the
// returned reader; the view does so when it is dismissed.
//
// This is deliberately not informer-backed: log lines cannot be re-read from a
// cache, so the stream itself is the source of truth.
func (s *PodSource) Logs(ctx context.Context, req core.LogRequest) (rc io.ReadCloser, err error) {
	logger := s.logger.With(
		zap.String("method", "Logs"),
		zap.String("namespace", req.Namespace),
		zap.String("pod", req.Pod),
		zap.String("container", req.Container),
	)

	tail := req.TailLines
	if tail <= 0 {
		tail = defaultTailLines
	}

	options := &corev1.PodLogOptions{
		Container: req.Container,
		Follow:    req.Follow,
		TailLines: &tail,
	}

	rc, err = s.client.CoreV1().Pods(req.Namespace).GetLogs(req.Pod, options).Stream(ctx)
	if err != nil {
		err = fmt.Errorf("error-opening-log-stream :%w", err)
		logger.Error("error-opening-log-stream", zap.Error(err))
		return rc, err
	}

	logger.Info("log-stream-opened", zap.Bool("follow", req.Follow))
	return rc, err
}
