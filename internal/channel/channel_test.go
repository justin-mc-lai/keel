package channel

import (
	"path/filepath"
	"testing"
)

func TestLocalChannelWritesOnEnd(t *testing.T) {
	c, err := NewLocal(filepath.Join(t.TempDir(), "channel.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(Record{TicketID: "t1", Source: "manual", Result: "ok"}); err != nil {
		t.Fatal(err)
	}
	recs, err := c.Records()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].TicketID != "t1" || recs[0].Result != "ok" {
		t.Fatalf("records = %+v", recs)
	}
	if recs[0].At == 0 {
		t.Fatal("timestamp not stamped")
	}
}
