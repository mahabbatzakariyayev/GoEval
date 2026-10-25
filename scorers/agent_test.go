package scorers

import (
	"context"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func TestToolMetrics(t *testing.T) {
	a := goeval.ToolCall{Name: "search", Arguments: map[string]any{"q": "Paris"}}
	b := goeval.ToolCall{Name: "book", Arguments: map[string]any{"city": "Paris", "people": 2}}
	s := goeval.Sample{Messages: []goeval.Message{{Role: "assistant", ToolCalls: []goeval.ToolCall{b, a, a}}}, ReferenceToolCalls: []goeval.ToolCall{a, b}}
	assertValue(t, ToolCallF1(), s, 1)
	assertValue(t, ToolCallAccuracy(ToolAccuracyOptions{}), s, 0)
	s.Messages[0].ToolCalls = []goeval.ToolCall{b, a}
	assertValue(t, ToolCallAccuracy(ToolAccuracyOptions{}), s, 0)
	assertValue(t, ToolCallAccuracy(ToolAccuracyOptions{FlexibleOrder: true}), s, 1)
	s.Messages[0].ToolCalls = []goeval.ToolCall{a, {Name: "book", Arguments: map[string]any{"city": "Paris", "people": 3}}}
	assertValue(t, ToolCallAccuracy(ToolAccuracyOptions{}), s, 0.75)
	assertValue(t, ToolCallF1(), s, 0.5)
	empty := goeval.Sample{Messages: []goeval.Message{{Role: "assistant"}}, ReferenceToolCalls: []goeval.ToolCall{}}
	assertValue(t, ToolCallAccuracy(ToolAccuracyOptions{}), empty, 1)
	assertValue(t, ToolCallF1(), empty, 0)
}
func TestAgentGoalAndTopicStages(t *testing.T) {
	llm, done := scripted(t, []step{{"goal_accuracy.InferGoalOutcomePrompt", map[string]string{"workflow": "Human: book\nAI: booked"}, `{"user_goal":"book","end_state":"booked"}`}, {"goal_accuracy.CompareOutcomePrompt", map[string]string{"desired_outcome": "a booking", "arrived_outcome": "booked"}, `{"verdict":"1","reason":"done"}`}})
	defer done()
	assertValue(t, AgentGoalAccuracyWithReference(llm), goeval.Sample{Messages: []goeval.Message{{Role: "user", Content: "book"}, {Role: "assistant", Content: "booked"}}, Reference: "a booking"}, 1)
	topic, topicDone := scripted(t, []step{{"topic_adherence.TopicExtractionPrompt", nil, `{"topics":["physics","politics"]}`}, {"topic_adherence.TopicRefusedPrompt", nil, `{"refused_to_answer":false}`}, {"topic_adherence.TopicRefusedPrompt", nil, `{"refused_to_answer":true}`}, {"topic_adherence.TopicClassificationPrompt", nil, `{"classifications":[true,false]}`}})
	defer topicDone()
	score, err := TopicAdherence(topic, "precision").Score(context.Background(), goeval.Sample{Messages: []goeval.Message{{Role: "assistant", Content: "physics"}}, ReferenceTopics: []string{"physics"}})
	if err != nil || score.Value < 0.999 {
		t.Fatalf("%+v %v", score, err)
	}
}
