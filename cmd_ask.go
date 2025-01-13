package main

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

func (b *Bot) handleAskCommand(e *gateway.InteractionCreateEvent) error {
	member, err := b.s.Member(e.GuildID, e.Member.User.ID)
	if err != nil {
		b.respondError(e, "Failed to get member")
		return err
	}
	perms, err := b.s.Permissions(e.ChannelID, member.User.ID)
	if err != nil || !perms.Has(discord.PermissionManageChannels) {
		b.respondError(e, "You need to have the Manage Channels permission to create questions")
		return err
	}

	data := e.Data.(*discord.CommandInteraction)
	question := data.Options[0].String()
	options := make([]string, 0)
	var answerId = 0

	// Collect options
	for i := 1; i < len(data.Options); i++ {
		if data.Options[i].Name == "answer" {
			// customAnswer, _ = data.Options[i].BoolValue()
		} else if data.Options[i].Name == "answer_id" {
			aId, _ := data.Options[i].IntValue()
			answerId = int(aId)
		} else if data.Options[i].String() != "" {
			options = append(options, data.Options[i].String())
		}
	}

	if len(question) < 1 {
		b.respondError(e, "Please provide a question")
		return nil
	}

	if len(options) < 1 {
		b.respondError(e, "Please provide options")
		return nil
	}

	q := &Question{
		CreatorID: int64(e.Member.User.ID),
		GuildID:   int64(e.GuildID),
		Question:  question,
		Options:   options,
		Answer:    int64(answerId),
	}

	d := QuestionDraft{
		Question:   q,
		DraftID:    rand.Int64(),
		lastEdited: time.Now(),
	}

	for _, ok := drafts[d.DraftID]; ok; {
		d.DraftID = rand.Int64()
	}

	drafts[d.DraftID] = &d

	// Create buttons
	components := make([]discord.Component, len(options), len(options)+2)
	for i, opt := range options {
		components[i] = &discord.ButtonComponent{
			Style:    discord.PrimaryButtonStyle(),
			CustomID: discord.ComponentID(fmt.Sprintf("p_opt_%d", i)),
			Label:    opt,
		}
	}

	components = append(
		components,
		&discord.ButtonComponent{
			CustomID: discord.ComponentID(fmt.Sprintf("ask_%d", d.DraftID)),
			Label:    "✅",
			Style:    discord.SecondaryButtonStyle(),
		},
		&discord.ButtonComponent{
			CustomID: discord.ComponentID(fmt.Sprintf("cancel_ask_%d", d.DraftID)),
			Label:    "❌",
			Style:    discord.DangerButtonStyle(),
		},
	)

	// Send poll message
	_, err = b.s.SendMessageComplex(e.ChannelID, api.SendMessageData{
		Content: "## Preview\n" + question,
		Components: discord.Components(
			components...,
		),
	})
	if err != nil {
		b.respondError(e, "Failed to announce question")
		return err
	}

	return nil
}
