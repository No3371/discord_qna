package main

import (
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
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

	
    qId, err := data.Options.Find("question_id").IntValue()
    if err != nil {
        b.respondError(e, "Invalid question ID")
        return err
    }

    err = b.postQuestion(qId, int64(e.ChannelID))
    if err != nil {
        b.respondError(e, "Failed to post question")
        return err
    }

    b.respond(e, "🆗", discord.EphemeralMessage)

	return nil
}