package cmd

import (
	"strings"
	"testing"
)

// Every listed topic has text, every text file is listed, and completion
// offers exactly the listed topics.
func TestGuideTopicsMatchFiles(t *testing.T) {
	listed := map[string]bool{}
	for _, topic := range guideTopics {
		listed[topic.name] = true
		text, err := guideTopicText(topic.name)
		if err != nil || !strings.HasPrefix(text, "pudl guide "+topic.name+" — ") {
			t.Errorf("topic %q: text missing or misheaded (err %v)", topic.name, err)
		}
	}
	entries, err := guideText.ReadDir("guide")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".txt")
		if !listed[name] {
			t.Errorf("guide/%s is not listed in guideTopics", e.Name())
		}
	}
	if len(entries) != len(guideTopics) {
		t.Errorf("%d topic files, %d listed topics", len(entries), len(guideTopics))
	}
	names, _ := guideCmd.ValidArgsFunction(guideCmd, nil, "")
	if len(names) != len(guideTopics) {
		t.Errorf("completion offers %v", names)
	}
	if _, err := guideTopicText("memory"); err == nil {
		t.Error("unknown topic accepted")
	}
}
