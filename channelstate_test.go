package dstate

import (
	"sync"
	"testing"
	"time"
)

var ReferenceTime = time.Date(2019, 1, 1, 1, 0, 0, 0, time.UTC)

func createMockCS(numMessages int) *ChannelState {
	var owner sync.RWMutex
	cs := &ChannelState{
		Owner:    &owner,
		Messages: make([]*MessageState, numMessages),
	}

	for i := 0; i < numMessages; i++ {
		t := ReferenceTime.Add(time.Minute * time.Duration(i))

		cs.Messages[i] = &MessageState{
			ID:            int64(i),
			Content:       "haha",
			ParsedCreated: t,
		}
	}

	return cs
}

func TestChannelStateMaxMessages(t *testing.T) {
	cs := createMockCS(10)

	cs.UpdateMessages(true, 5, time.Time{})

	if len(cs.Messages) != 5 {
		t.Errorf("Len is %d but expected %d", len(cs.Messages), 5)
	}

	for i, v := range cs.Messages {
		if v == nil {
			t.Errorf("Index %d is nil!", i)
		}
	}
}

func TestChannelStateMaxMessages2(t *testing.T) {
	cs := createMockCS(10)

	cs.UpdateMessages(true, 10, time.Time{})

	if len(cs.Messages) != 10 {
		t.Errorf("Len is %d but expected %d", len(cs.Messages), 10)
	}

	for i, v := range cs.Messages {
		if v == nil {
			t.Errorf("Index %d is nil!", i)
		}
	}
}

func TestChannelStateMaxMessages3(t *testing.T) {
	cs := createMockCS(10)

	cs.UpdateMessages(true, 0, time.Time{})

	if len(cs.Messages) != 0 {
		t.Errorf("Len is %d but expected %d", len(cs.Messages), 0)
	}

	for i, v := range cs.Messages {
		if v == nil {
			t.Errorf("Index %d is nil!", i)
		}
	}
}

func TestChannelStateMaxMessages4(t *testing.T) {
	cs := createMockCS(10)

	cs.UpdateMessages(true, 1, time.Time{})

	if len(cs.Messages) != 1 {
		t.Errorf("Len is %d but expected %d", len(cs.Messages), 1)
	}

	for i, v := range cs.Messages {
		if v == nil {
			t.Errorf("Index %d is nil!", i)
		}
	}
}

func TestChannelStateMaxMessageAge(t *testing.T) {
	cs := createMockCS(10)

	cs.UpdateMessages(true, -1, ReferenceTime.Add(time.Minute*2))

	if len(cs.Messages) != 8 {
		t.Errorf("Len is %d but expected %d", len(cs.Messages), 8)
	}

	for i, v := range cs.Messages {
		if v == nil {
			t.Errorf("Index %d is nil!", i)
		}
	}
}

func TestChannelStateMaxMessageAge2(t *testing.T) {
	cs := createMockCS(10)

	cs.UpdateMessages(true, -1, ReferenceTime.Add(time.Minute*20))

	if len(cs.Messages) != 0 {
		t.Errorf("Len is %d but expected %d", len(cs.Messages), 0)
	}

	for i, v := range cs.Messages {
		if v == nil {
			t.Errorf("Index %d is nil!", i)
		}
	}
}

func TestChannelStateMaxMessageAge3(t *testing.T) {
	cs := createMockCS(10)

	cs.UpdateMessages(true, -1, ReferenceTime.Add(time.Minute*9))

	if len(cs.Messages) != 1 {
		t.Errorf("Len is %d but expected %d", len(cs.Messages), 1)
	}

	for i, v := range cs.Messages {
		if v == nil {
			t.Errorf("Index %d is nil!", i)
		}
	}
}

func TestChannelStateMessagesCombined(t *testing.T) {
	cs := createMockCS(10)

	cs.UpdateMessages(true, 5, ReferenceTime.Add(time.Minute*2))

	if len(cs.Messages) != 5 {
		t.Errorf("Len is %d but expected %d", len(cs.Messages), 5)
	}

	for i, v := range cs.Messages {
		if v == nil {
			t.Errorf("Index %d is nil!", i)
		}
	}
}

func TestChannelStateMessagesCombined2(t *testing.T) {
	cs := createMockCS(10)

	cs.UpdateMessages(true, 5, ReferenceTime.Add(time.Minute*8))

	if len(cs.Messages) != 2 {
		t.Errorf("Len is %d but expected %d", len(cs.Messages), 2)
	}

	for i, v := range cs.Messages {
		if v == nil {
			t.Errorf("Index %d is nil!", i)
		}
	}
}
