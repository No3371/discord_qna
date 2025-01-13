package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

func (b *Bot) handleListCommand(e *gateway.InteractionCreateEvent) error {
    data := e.Data.(*discord.CommandInteraction)
    
    // Check permissions
    member, err := b.s.Member(e.GuildID, e.Member.User.ID)
    if err != nil {
        b.respondError(e, "Failed to get member")
        return err
    }
    perms, err := b.s.Permissions(e.ChannelID, member.User.ID)
    if err != nil || !perms.Has(discord.PermissionManageChannels) {
        b.respondError(e, "You need to have the Manage Channels permission to view questions")
        return err
    }

    count := int64(10)
    if opt := data.Options.Find("count"); opt.Name != "" {
        count, err = opt.IntValue()
        if err != nil {
            b.respondError(e, "Invalid count value")
            return err
        }
    }

    showToEveryone := false
    if opt := data.Options.Find("public"); opt.Name != "" {
        showToEveryone, err = opt.BoolValue()
        if err != nil {
            b.respondError(e, "Invalid public value")
            return err
        }
    }

    // Get recent questions
    rows, err := b.db.Query(`
        SELECT id, creator_id, question, created_at, is_closed
        FROM questions 
        WHERE guild_id = ?
        ORDER BY created_at ASC
        LIMIT ?`,
        e.GuildID.String(),
        count,
    )
    if err != nil {
        b.respondError(e, "Failed to get questions")
        return err
    }
    defer rows.Close()

    var result strings.Builder
    result.WriteString("**Recent Questions**\n")
    
    i := 1
    for rows.Next() {
        var id int64
        var creatorId int64
        var question string
        var creationTime time.Time
        var isClosed bool
        err := rows.Scan(&id, &creatorId, &question, &creationTime, &isClosed)
        if err != nil {
            continue
        }

        if isClosed {
            result.WriteString("\n🔒")
        } else {
            result.WriteString("\n🔓")
        }
        
        if strings.Contains(question, "\n") {
            result.WriteString(fmt.Sprintf("**#%d**\n%s (<@%d> <t:%d:R>)\n", 
                id, question, creatorId, creationTime.Unix()))
        } else {
        result.WriteString(fmt.Sprintf("**#%d**: %s (<@%d> <t:%d:R>)\n", 
            id, question, creatorId, creationTime.Unix()))
        }
        i++
    }

    if i == 1 {
        b.respond(e, "No question", discord.EphemeralMessage)
        return err
    }

    if showToEveryone {
        b.respond(e, result.String(), 0)
    } else {
        b.respond(e, result.String(), discord.EphemeralMessage)
    }

    return nil
}
