package main

import (
	"fmt"
	// "log"
	"strings"
	

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)


func (b *Bot) handleParseCommand(e *gateway.InteractionCreateEvent) error {
    data := e.Data.(*discord.CommandInteraction)

    // Check permissions
    member, err := b.s.Member(e.GuildID, e.Member.User.ID)
    if err != nil {
        b.respondError(e, "Failed to get member")
        return err
    }
    perms, err := b.s.Permissions(e.ChannelID, member.User.ID)
    if err != nil || !perms.Has(discord.PermissionManageChannels) {
        b.respondError(e, "You need to have the Manage Channels permission to create polls")
        return err
    }

    // for _, m := range data.Resolved.Messages {
    //     log.Printf(m.Content)
    // }

    if len(data.Resolved.Messages) == 0 {
        b.respondError(e, "No message provided")
        return nil
    }

    // m, err := b.s.Message(e.ChannelID, discord.MessageID(data.TargetID))
    // if err != nil {
    //     b.respondError(e, "Failed to get message")
    //     return err
    // }

    // log.Printf(def)

    questions, err := parseQuestionMarkdown(data.Resolved.Messages[data.TargetMessageID()].Content)
    if err != nil {
        b.respondError(e, "Failed to parse question")
        return err
    }

    qIds := []int64{}

    for _, qDraft := range questions {
        qDraft.CreatorID = int64(e.Member.User.ID)
        qDraft.GuildID = int64(e.GuildID)

        q, err := b.insertQuestion(qDraft)
        if err != nil {
            b.respondError(e, "Failed to insert question")
            return err
        }

        qIds = append(qIds, q.QID)

        if err := b.postQuestion(q.QID, int64(e.ChannelID)); err != nil {
            b.respondError(e, "Failed to post question")
            return err
        }
    }

    b.respond(e, "Posted: "+strings.Trim(strings.Join(strings.Fields(fmt.Sprint(qIds)), ","), "[]"), discord.EphemeralMessage)

    return nil
}

func parseQuestionMarkdown (md string) ([]*Question, error) {
    // Split into lines
    lines := strings.Split(md, "\n")
    
    if len(lines) == 0 {
        return nil, fmt.Errorf("empty markdown")
    }

    questions := []*Question{}
    q := &Question{}
    var inQuestion bool
    var inOptions bool

    // Parse lines
    for _, line := range lines {
        // Skip empty lines
        if strings.TrimSpace(line) == "" {
            inQuestion = false
            inOptions = false
            continue
        }

        // If line starts with "- ", it's an option
        if inQuestion && strings.HasPrefix(strings.TrimSpace(line), "- ") {
            inOptions = true
            option := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
            if option != "" {
                if strings.HasPrefix(option, "[O]") {
                    q.Answer = int64(len(q.Options))
                    q.Options = append(q.Options, option[3:])
                } else {
                    q.Options = append(q.Options, option)
                }
            }
        } else {
            if inOptions {
                return nil, fmt.Errorf("question text must be before options")
            }
            if !inQuestion {
                q = new(Question)
                questions = append(questions, q)
            }
            // It's part of the question text
            q.Question += line + "\n"
            inQuestion = true
        }
    }

    if len(questions) == 0 {
        return nil, fmt.Errorf("no question")
    }

    for _, q := range questions {
        if len(q.Options) == 0 {
            return nil, fmt.Errorf("no option")
        }
    }

    return questions, nil
}