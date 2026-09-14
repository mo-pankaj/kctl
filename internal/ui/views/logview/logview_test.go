package logview_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/core/mocks"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
	"github.com/mo-pankaj/kctl/internal/ui/views/logview"
)

// blockingReader yields its lines, then blocks until closed, imitating a follow
// stream that stays open.
type blockingReader struct {
	reader io.Reader
	done   chan struct{}
}

func newBlockingReader(body string) *blockingReader {
	return &blockingReader{reader: strings.NewReader(body), done: make(chan struct{})}
}

func (b *blockingReader) Read(p []byte) (n int, err error) {
	n, err = b.reader.Read(p)
	if err == io.EOF {
		<-b.done
		return 0, io.EOF
	}

	return n, err
}

func (b *blockingReader) Close() error {
	close(b.done)
	return nil
}

func newLogView(t *testing.T, body string) logview.Model {
	t.Helper()

	ctrl := gomock.NewController(t)
	reader := mocks.NewMockPodReader(ctrl)

	reader.EXPECT().
		Logs(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, core.LogRequest) (io.ReadCloser, error) {
			return newBlockingReader(body), nil
		}).
		AnyTimes()

	req := core.LogRequest{Namespace: "ns1", Pod: "api-1", Container: "app", Follow: true}

	return logview.New(zap.NewNop(), theme.New(), keys.Default(), reader, req)
}

func TestLogViewRendersStreamedLines(t *testing.T) {
	body := "first line\nsecond line\nthird line\n"

	tm := teatest.NewTestModel(t, newLogView(t, body), teatest.WithInitialTermSize(120, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("first line")) && bytes.Contains(b, []byte("third line"))
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
}

func TestLogViewHeaderNamesThePodAndContainer(t *testing.T) {
	tm := teatest.NewTestModel(t, newLogView(t, "hello\n"), teatest.WithInitialTermSize(120, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("api-1")) && bytes.Contains(b, []byte("app"))
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
}

func TestFToggleFollowIsReflectedInTheHeader(t *testing.T) {
	tm := teatest.NewTestModel(t, newLogView(t, "hello\n"), teatest.WithInitialTermSize(120, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("follow"))
	}, teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyPressMsg{Code: 'f', Text: "f"})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("paused"))
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
}
