package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/dlclark/regexp2"
	_ "modernc.org/sqlite"
)

var responseButtonRegex = regexp2.MustCompile(`opt_(\d+)_(\d)`, regexp2.None)

type QuestionDraft struct {
	*Question
	DraftID    int64
	lastEdited time.Time
}

type Question struct {
	QID        int64     `db:"id"`
	CreatorID  int64     `db:"creator_id"`
	GuildID    int64     `db:"guild_id"`
	Question   string    `db:"question"`
	OptionsStr string    `db:"options"`
	Answer     int64     `db:"answer_id"`
	CreatedAt  time.Time `db:"created_at"`
	IsClosed   bool      `db:"is_closed"`
	IsAnon   bool      `db:"is_anon"`
	Options    []string
}

func (b *Bot) queryQuestion(qId int64) (*Question, error) {
	q := Question{}
	err := b.db.QueryRow(
		"SELECT id, creator_id, guild_id, question, options, answer_id, created_at, is_closed, is_anon FROM questions WHERE id = ?",
		qId,
	).Scan(&q.QID, &q.CreatorID, &q.GuildID, &q.Question, &q.OptionsStr, &q.Answer, &q.CreatedAt, &q.IsClosed, &q.IsAnon)
	if err != nil {
		return nil, fmt.Errorf("Question not found: %w", err)
	}

	if q.OptionsStr != "" {
		q.Options = strings.Split(q.OptionsStr, "|")
	}

	return &q, nil
}

func (b *Bot) insertQuestion(q *Question) (*Question, error) {
	q.OptionsStr = strings.Join(q.Options, "|")
	result, err := b.db.Exec(
		"INSERT INTO questions (creator_id, guild_id, question, options, answer_id, is_anon) VALUES (?, ?, ?, ?, ?, ?)",
		q.CreatorID,
		q.GuildID,
		q.Question,
		q.OptionsStr,
		q.Answer,
		q.IsAnon,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store question: %w", err)
	}

	questionID, _ := result.LastInsertId()
	err = b.db.QueryRow(
		"SELECT id, created_at, is_closed FROM questions WHERE id = ?",
		questionID,
	).Scan(&q.QID, &q.CreatedAt, &q.IsClosed)
	if err != nil {
		return nil, fmt.Errorf("Question not found: %w", err)
	}

	return q, nil
}

var drafts = make(map[int64]*QuestionDraft)

func (b *Bot) handleInteraction(e *gateway.InteractionCreateEvent) {
	defer func() {
		err := recover()
		if err != nil {
			b.respondError(e, fmt.Sprintf("Unhandled error: %v", err))
		}
	}()
	var err error
	switch data := e.Data.(type) {
	case *discord.CommandInteraction:
		switch data.Name {
		case "ask":
			err = b.handleAskCommand(e)
		case "result":
			err = b.handleResultCommand(e)
		case "close":
			err = b.handleCloseCommand(e)
		case "post":
			err = b.handlePostCommand(e)
		case "analyze":
			err = b.handleAnalyzeCommand(e)
		case "list":
			err = b.handleListCommand(e)
		case "Make questions":
			err = b.handleParseCommand(e)
		}
	case *discord.ButtonInteraction:
		err = b.handleButtonClick(e)
	}

	if err != nil {
		log.Printf("%v", err)
	}

	for _, draft := range drafts {
		if time.Since(draft.lastEdited) > time.Hour*24 {
			delete(drafts, draft.DraftID)
		}
	}
}

// func ParseQuestionMarkdown (md string) (*Question, error) {

// }

func (b *Bot) preparePost(q *Question) (api.SendMessageData, error) {

	content := q.Question + fmt.Sprintf("-# \\#%d", q.QID)
	
	if q.IsAnon {
		content = "[㊙️ Anonymous]\n" + content
	}

	components := make([]discord.Component, len(q.Options))
	for i, opt := range q.Options {
		components[i] = &discord.ButtonComponent{
			CustomID: discord.ComponentID(fmt.Sprintf("opt_%d_%d", q.QID, i)),
			Label:    opt,
			Style:    discord.PrimaryButtonStyle(),
		}
	}

	return api.SendMessageData{
		Content: content,
		Components: discord.Components(
			components...,
		),
	}, nil
}

func (b *Bot) postQuestion(qId int64, channelId int64) error {
	q, err := b.queryQuestion(qId)
	if err != nil {
		return fmt.Errorf("Question not found: %w", err)
	}

	msgData, err := b.preparePost(q)
	if err != nil {
		return err
	}
	// Send poll message
	_, err = b.s.SendMessageComplex(discord.ChannelID(channelId), msgData)
	if err != nil {
		return fmt.Errorf("Failed to post question:: %w", err)
	}

	return nil
}

func (b *Bot) handleButtonClick(e *gateway.InteractionCreateEvent) error {
	data := e.Data.(*discord.ButtonInteraction)

	if strings.HasPrefix(string(data.CustomID), "ask_") {
		askIdStr, valid := strings.CutPrefix(string(data.CustomID), "ask_")
		if !valid {
			return fmt.Errorf("Invalid ask ID")
		}

		askId, err := strconv.ParseInt(askIdStr, 10, 64)
		if err != nil {
			return err
		}

		d, ok := drafts[askId]
		if !ok {
			b.respondError(e, "Draft not found")
			return err
		}

		q, err := b.insertQuestion(d.Question)
		if err != nil {
			b.respondError(e, "Failed to insert question")
			return err
		}

		if err := b.postQuestion(q.QID, int64(e.ChannelID)); err != nil {
			b.respondError(e, "Failed to post question")
			return err
		}
		b.respond(e, fmt.Sprintf("🆗"), discord.EphemeralMessage)
	} else if strings.HasPrefix(string(data.CustomID), "cancel_ask_") {
		askIdStr, valid := strings.CutPrefix(string(data.CustomID), "cancel_ask_")
		if !valid {
			return fmt.Errorf("Invalid ask ID")
		}

		askId, err := strconv.ParseInt(askIdStr, 10, 64)
		if err != nil {
			return err
		}

		delete(drafts, askId)
		b.s.DeleteMessage(e.ChannelID, e.Message.ID, "")
		b.respond(e, fmt.Sprintf("Cancelled!"), discord.EphemeralMessage)
	} else if strings.HasPrefix(string(data.CustomID), "opt_") {
		match, err := responseButtonRegex.FindStringMatch(string(data.CustomID))
		if err != nil {
			return err
		}
		if match == nil {
			return fmt.Errorf("invalid response button ID")
		}

		g := match.Groups()
		if len(g) != 3 {
			return fmt.Errorf("invalid response button ID")
		}

		qId, err := strconv.ParseInt(g[1].String(), 10, 64)
		if err != nil {
			return err
		}
		rId, err := strconv.ParseInt(g[2].String(), 10, 64)
		if err != nil {
			return err
		}

		// Extract question ID from message
		var questionID int64
		err = b.db.QueryRow("SELECT id FROM questions WHERE id = ? AND is_closed = FALSE", qId).Scan(&questionID)
		if err != nil {
			b.respondError(e, "Poll not found or closed")
			return err
		}

		// Record response
		_, err = b.db.Exec(
			"INSERT OR REPLACE INTO responses (question_id, user_id, choice) VALUES (?, ?, ?)",
			questionID,
			e.Member.User.ID,
			rId,
		)
		if err != nil {
			b.respondError(e, "Failed to record response")
			return err
		}
		b.respond(e, fmt.Sprintf("🆗"), discord.EphemeralMessage)
	}

	return nil
}

func (b *Bot) respond(e *gateway.InteractionCreateEvent, content string, flags discord.MessageFlags) {
	err := b.s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
		Type: api.MessageInteractionWithSource,
		Data: &api.InteractionResponseData{
			Content: option.NewNullableString(content),
			Flags:   flags,
		},
	})
	if err != nil {
		log.Printf("Failed to respond to interaction: %v", err)
	}
}

func (b *Bot) respondError(e *gateway.InteractionCreateEvent, message string) {
	err := b.s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
		Type: api.MessageInteractionWithSource,
		Data: &api.InteractionResponseData{
			Content: option.NewNullableString("❌" + message),
			Flags:   discord.EphemeralMessage,
		},
	})
	if err != nil {
		log.Printf("Failed to respond to interaction: %v", err)
	}
}

func parseIds(ids string) []int64 {
	var result []int64
	for _, id := range strings.Split(ids, ",") {
		i, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			continue
		}
		result = append(result, i)
	}
	return result
}
