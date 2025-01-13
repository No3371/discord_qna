package main

import (
	"fmt"
	// "strings"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)


func (b *Bot) handleCloseCommand(e *gateway.InteractionCreateEvent) error {

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

    data := e.Data.(*discord.CommandInteraction)	
    qIds := parseIds(data.Options.Find("question_ids").String())

	if len(qIds) == 0 {
		b.respondError(e, "No question ID provided")
		return nil
	}

	// inClause := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(qIds)), ","), "[]")

    // // Close the questions
    // r, err := b.db.Exec("UPDATE questions SET is_closed = TRUE WHERE id IN (?) AND guild_id = ? AND is_closed = FALSE", inClause, e.GuildID.String())
    // if err != nil {
    //     b.respondError(e, "Failed to close questions")
    //     return err
    // }

	// rows, err := r.RowsAffected()
	// if err != nil {
	// 	b.respondError(e, "Failed to close questions")
	// 	return err
	// }

	count := 0
	for _, qId := range qIds {
		r, err := b.db.Exec("UPDATE questions SET is_closed = TRUE WHERE id = ? AND guild_id = ? AND is_closed = FALSE", qId, e.GuildID.String())
		if err != nil {
			b.respondError(e, "Failed to close questions")
			return err
		}

		rows, err := r.RowsAffected()
		if err != nil {
			b.respondError(e, "Failed to close questions")
			return err
		}
		if rows == 1 {
			count++
		}
	}

    b.respond(e, fmt.Sprintf("Closed %d/%d questions", count, len(qIds)), discord.EphemeralMessage)

	return nil
}