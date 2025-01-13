package main

import (
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)


func (b *Bot) handlePostCommand(e *gateway.InteractionCreateEvent) error {
    data := e.Data.(*discord.CommandInteraction)

    // Check permissions
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

    qIds := parseIds(data.Options.Find("question_ids").String())

	if len(qIds) == 0 {
		b.respondError(e, "No question ID provided")
		return nil
	}
    
    err = b.s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
		Type: api.DeferredMessageInteractionWithSource,
        Data: &api.InteractionResponseData{
            Flags: discord.EphemeralMessage,
        },
	})
    if err != nil {
        return err
    }

    for _, qId := range qIds {
        err = b.postQuestion(qId, int64(e.ChannelID))
        if err != nil {
            _, err = b.s.EditInteractionResponse(e.AppID, e.Token, api.EditInteractionResponseData{
                Content: option.NewNullableString("❌Failed to post question"),
            })
            return err
        }
        <-time.NewTimer(time.Second).C
    }

    _, err = b.s.EditInteractionResponse(e.AppID, e.Token, api.EditInteractionResponseData{
        Content: option.NewNullableString("🆗"),
	})
    if err != nil {
        return err
    }

	return nil
}