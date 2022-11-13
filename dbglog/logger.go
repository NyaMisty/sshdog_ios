package dbglog

import (
	"log"
	"os"
)

type DbgLogger struct {
	*log.Logger
	Enable bool
}

func (d *DbgLogger) Debug(format string, args ...interface{}) {
	if d.Enable {
		//msg := fmt.Sprintf(format, args...)
		//fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", msg)
		d.Printf(" "+format, args...)
	}
}
func (d *DbgLogger) NewPrefix(newprefix string) {
	d.Logger = d.WithPrefix(newprefix).Logger
}

func (d *DbgLogger) WithPrefix(newprefix string) DbgLogger {
	return DbgLogger{log.New(os.Stderr, d.Prefix()+newprefix, d.Flags()|log.Lmsgprefix), true}
}

var dbg = &DbgLogger{log.New(os.Stderr, "[DEBUG]", log.LstdFlags|log.Lmsgprefix), true}
var Dbg = dbg
