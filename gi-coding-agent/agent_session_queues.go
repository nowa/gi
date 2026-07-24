package gicodingagent

import llm "github.com/nowa/gi/gi-llm-provider"

type agentSessionPromptQueueKind uint8

const (
	agentSessionSteeringQueue agentSessionPromptQueueKind = iota
	agentSessionFollowUpQueue
)

func (q *agentSessionQueueState) enqueuePrompt(
	kind agentSessionPromptQueueKind,
	message QueuedUserMessage,
) {
	message = cloneQueuedUserMessage(message)
	q.mu.Lock()
	defer q.mu.Unlock()
	switch kind {
	case agentSessionFollowUpQueue:
		q.followUpMessages = append(q.followUpMessages, message.Text)
		q.followUp = append(q.followUp, message)
	default:
		q.steeringMessages = append(q.steeringMessages, message.Text)
		q.steering = append(q.steering, message)
	}
}

func (q *agentSessionQueueState) takePrompt(
	kind agentSessionPromptQueueKind,
	mode string,
) []QueuedUserMessage {
	q.mu.Lock()
	defer q.mu.Unlock()

	queue := &q.steering
	texts := &q.steeringMessages
	if kind == agentSessionFollowUpQueue {
		queue = &q.followUp
		texts = &q.followUpMessages
	}
	if len(*queue) == 0 {
		return nil
	}

	count := 1
	if mode == "all" {
		count = len(*queue)
	}
	messages := cloneQueuedUserMessages((*queue)[:count])
	*queue = cloneQueuedUserMessages((*queue)[count:])
	if len(*texts) <= count {
		*texts = nil
	} else {
		*texts = append([]string(nil), (*texts)[count:]...)
	}
	return messages
}

func (q *agentSessionQueueState) hasPrompt(kind agentSessionPromptQueueKind) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if kind == agentSessionFollowUpQueue {
		return len(q.followUp) > 0
	}
	return len(q.steering) > 0
}

func (q *agentSessionQueueState) pendingPromptCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.steeringMessages) + len(q.followUpMessages)
}

func (q *agentSessionQueueState) promptMessages(
	kind agentSessionPromptQueueKind,
) []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if kind == agentSessionFollowUpQueue {
		return append([]string(nil), q.followUpMessages...)
	}
	return append([]string(nil), q.steeringMessages...)
}

func (q *agentSessionQueueState) promptQueue(
	kind agentSessionPromptQueueKind,
) []QueuedUserMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	if kind == agentSessionFollowUpQueue {
		return cloneQueuedUserMessages(q.followUp)
	}
	return cloneQueuedUserMessages(q.steering)
}

func (q *agentSessionQueueState) promptSnapshot() (steering, followUp []string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.steeringMessages...),
		append([]string(nil), q.followUpMessages...)
}

func (q *agentSessionQueueState) clearPrompts() (steering, followUp []string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	steering = append([]string(nil), q.steeringMessages...)
	followUp = append([]string(nil), q.followUpMessages...)
	q.steeringMessages = nil
	q.followUpMessages = nil
	q.steering = nil
	q.followUp = nil
	return steering, followUp
}

func (q *agentSessionQueueState) enqueueNextTurn(message QueuedCustomMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pendingNextTurn = append(
		q.pendingNextTurn,
		cloneQueuedCustomMessage(message),
	)
}

func (q *agentSessionQueueState) takeNextTurn() []QueuedCustomMessage {
	q.mu.Lock()
	defer q.mu.Unlock()
	messages := cloneQueuedCustomMessages(q.pendingNextTurn)
	q.pendingNextTurn = nil
	return messages
}

func (q *agentSessionQueueState) enqueueAgentMessage(message llm.Message) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.agentMessages = append(q.agentMessages, cloneSessionMessage(message))
}

func (q *agentSessionQueueState) hasAgentMessages() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.agentMessages) > 0
}

func (q *agentSessionQueueState) takeAgentMessages() []llm.Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	messages := cloneSessionMessages(q.agentMessages)
	q.agentMessages = nil
	return messages
}

func (q *agentSessionQueueState) enqueueBashMessage(message map[string]any) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pendingBashMessages = append(
		q.pendingBashMessages,
		cloneSessionMap(message),
	)
}

func (q *agentSessionQueueState) hasBashMessages() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pendingBashMessages) > 0
}

func (q *agentSessionQueueState) takeBashMessages() []map[string]any {
	q.mu.Lock()
	defer q.mu.Unlock()
	messages := make([]map[string]any, len(q.pendingBashMessages))
	for index, message := range q.pendingBashMessages {
		messages[index] = cloneSessionMap(message)
	}
	q.pendingBashMessages = nil
	return messages
}

func cloneQueuedUserMessage(message QueuedUserMessage) QueuedUserMessage {
	cloned := QueuedUserMessage{
		Text:   message.Text,
		Images: cloneSessionContent(message.Images),
	}
	if message.Custom != nil {
		custom := cloneQueuedCustomMessage(*message.Custom)
		cloned.Custom = &custom
	}
	return cloned
}

func cloneQueuedUserMessages(messages []QueuedUserMessage) []QueuedUserMessage {
	if messages == nil {
		return nil
	}
	cloned := make([]QueuedUserMessage, len(messages))
	for index, message := range messages {
		cloned[index] = cloneQueuedUserMessage(message)
	}
	return cloned
}

func cloneQueuedCustomMessage(message QueuedCustomMessage) QueuedCustomMessage {
	message.Content = cloneSessionValue(message.Content)
	message.Details = cloneSessionValue(message.Details)
	return message
}

func cloneQueuedCustomMessages(messages []QueuedCustomMessage) []QueuedCustomMessage {
	if messages == nil {
		return nil
	}
	cloned := make([]QueuedCustomMessage, len(messages))
	for index, message := range messages {
		cloned[index] = cloneQueuedCustomMessage(message)
	}
	return cloned
}

func cloneSessionMessages(messages []llm.Message) []llm.Message {
	if messages == nil {
		return nil
	}
	cloned := make([]llm.Message, len(messages))
	for index, message := range messages {
		cloned[index] = cloneSessionMessage(message)
	}
	return cloned
}
