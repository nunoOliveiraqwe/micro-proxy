package acme

import (
	"fmt"

	"github.com/go-acme/lego/v4/log"
	"go.uber.org/zap"
)

// zapStdLogger adapts a *zap.Logger to the lego StdLogger interface.
type zapStdLogger struct {
	l *zap.SugaredLogger
}

func (z *zapStdLogger) Fatal(args ...any)                 { z.l.Fatal(args...) }
func (z *zapStdLogger) Fatalln(args ...any)               { z.l.Fatal(args...) }
func (z *zapStdLogger) Fatalf(format string, args ...any) { z.l.Fatalf(format, args...) }
func (z *zapStdLogger) Print(args ...any)                 { z.l.Info(args...) }
func (z *zapStdLogger) Println(args ...any)               { z.l.Info(args...) }
func (z *zapStdLogger) Printf(format string, args ...any) { z.l.Infof(format, args...) }
func (z *zapStdLogger) String() string                    { return fmt.Sprintf("zapStdLogger{%v}", z.l) }

func registerLogger() {
	log.Logger = &zapStdLogger{l: zap.L().Sugar()}
}
