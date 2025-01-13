package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

func (b *Bot) handleResultCommand(e *gateway.InteractionCreateEvent) error {
	data := e.Data.(*discord.CommandInteraction)
	questionID, err := data.Options[0].IntValue()
	if err != nil {
		b.respondError(e, "Invalid question ID")
		return err
	}

	showToEveryone := false
	if opt := data.Options.Find("public"); opt.Name != "" {
		showToEveryone, err = opt.BoolValue()
		if err != nil {
			b.respondError(e, "Invalid public value")
			return err
		}
	}

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

	q, err := b.queryQuestion(questionID)
	if err != nil || q.GuildID != int64(e.GuildID) {
		b.respondError(e, "Poll not found")
		return err
	}

	// Get response counts
	rows, err := b.db.Query(`
        SELECT 
            choice,
            COUNT(*) as count,
            GROUP_CONCAT(user_id) as users,
            GROUP_CONCAT(unixepoch(responded_at)) as responded_ats
        FROM responses 
        WHERE question_id = ?
        GROUP BY choice
        ORDER BY MIN(responded_at)`,
		questionID,
	)
	if err != nil {
		b.respondError(e, "Failed to get results")
		return err
	}
	defer rows.Close()

	totalResponses := 0
	var result strings.Builder

	result.WriteString(fmt.Sprintf("**Question**\n%s\n\n", q.Question))
	for rows.Next() {
		var choice string
		var respTimes string
		var count int
		var users string
		rows.Scan(&choice, &count, &users, &respTimes)

		respTimesList := strings.Split(respTimes, ",")
		var respTime []time.Time = make([]time.Time, 0)
		for _, v := range respTimesList {
			t, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				log.Printf("Failed to parse response time: %v", err)
			}
			respTime = append(respTime, time.Unix(t, 0))
			respTimesList = respTimesList[1:]
		}

		choiceIdx, _ := strconv.Atoi(choice)
		optionText := q.Options[choiceIdx]
		totalResponses += count

		if q.Answer != -1 && choiceIdx == int(q.Answer) {
			result.WriteString("✅")
		}
		result.WriteString(fmt.Sprintf("**Option:** %s (%.1f%%)\n", optionText, float64(count)*100/float64(totalResponses)))
		for i, userID := range strings.Split(users, ",") {
			t := respTime[0]
			respTime = respTime[1:]
			result.WriteString(fmt.Sprintf("%d. <@%s> (<t:%d:R>)\n", i+1, userID, t.Unix()))
		}
	}

    if totalResponses == 0 {
        result.WriteString("❌ *No responses*\n")
    }

	result.WriteString(fmt.Sprintf("\nCreated by <@%d> at: <t:%d> (<t:%d:R>)\n", q.CreatorID, q.CreatedAt.Unix(), q.CreatedAt.Unix()))
	result.WriteString(fmt.Sprintf("Total responses: %d", totalResponses))
	if showToEveryone {
		b.respond(e, result.String(), 0)
	} else {
		b.respond(e, result.String(), discord.EphemeralMessage)
	}

	return nil
}
