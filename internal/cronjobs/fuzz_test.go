package cronjobs

import (
	"testing"
	"time"
)

func FuzzCronExpression(f *testing.F) {
	for _, v := range []string{"* * * * *", "0 2 * * *", "*/5 0-23 * * 1-5", "bad"} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v string) {
		schedule, err := Parse(v)
		if err == nil {
			_ = schedule.Next(time.Unix(0, 0).UTC())
		}
	})
}
