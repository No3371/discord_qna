package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

func (b *Bot) handleAnalyzeCommand(e *gateway.InteractionCreateEvent) error {
    data := e.Data.(*discord.CommandInteraction)
    
    // Check permissions
    member, err := b.s.Member(e.GuildID, e.Member.User.ID)
    if err != nil {
        b.respondError(e, "Failed to get member")
        return err
    }
    perms, err := b.s.Permissions(e.ChannelID, member.User.ID)
    if err != nil || !perms.Has(discord.PermissionManageChannels) {
        b.respondError(e, "You need to have the Manage Channels permission to view analysis")
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

    // Parse question IDs
    questionIDs := strings.Split(data.Options[0].String(), ",")
    var result strings.Builder
    
    // Track statistics
    type UserStats struct {
        correct int
        total   int
    }
    userStats := make(map[string]*UserStats)
    type QuestionStats struct {
        id            int64
        question      string
        correct       int
        total        int
        correctUsers []string
    }
    var questionStats []QuestionStats
    totalCorrect := 0
    totalAnswers := 0
    anyAnon := false

    // Analyze each question
    for _, qIDStr := range questionIDs {
        qID, err := strconv.ParseInt(strings.TrimSpace(qIDStr), 10, 64)
        if err != nil {
            continue
        }

        // Get question info
        var question string
        var options string
        var correctAnswerID int
        var guildId string
        var isClosed bool
        var isAnon bool
        err = b.db.QueryRow(
            "SELECT question, options, answer_id, guild_id, is_closed, is_anon FROM questions WHERE id = ?",
            qID,
        ).Scan(&question, &options, &correctAnswerID, &guildId, &isClosed, &isAnon)
        if err != nil {
            continue
        }

        if guildId != e.GuildID.String() {
            b.respondError(e, fmt.Sprintf("Q#%d is not your poll!", qID))
            return err
        }

        if isClosed {
            result.WriteString("\n🔒")
        } else {
            result.WriteString("\n🔓")
        }
        if isAnon {
            result.WriteString("㊙️")
            anyAnon = true
        }

        if strings.Contains(question, "\n") {
            result.WriteString(fmt.Sprintf("**#%d**\n%s\n", qID, question))
        } else {
            result.WriteString(fmt.Sprintf("**#%d**: %s\n", qID, question))
        }
        optionsList := strings.Split(options, "|")
        
        // Get responses
        rows, err := b.db.Query(`
            SELECT 
                choice,
                COUNT(*) as count,
                GROUP_CONCAT(user_id) as users
            FROM responses 
            WHERE question_id = ?
            GROUP BY choice
            ORDER BY choice`,
            qID,
        )
        if err != nil {
            continue
        }

        qStats := QuestionStats{
            id:       qID,
            question: question,
        }

        // Process responses
        totalResponses := 0
        optionCounts := make(map[int]int)
        optionUsers := make(map[int][]string)
        
        for rows.Next() {
            var choice string
            var count int
            var users string
            rows.Scan(&choice, &count, &users)
            
            choiceIdx, _ := strconv.Atoi(choice)
            optionCounts[choiceIdx] = count
            optionUsers[choiceIdx] = strings.Split(users, ",")
            totalResponses += count
        }
        rows.Close()

        // Show results for each option
        for i, opt := range optionsList {
            count := optionCounts[i]
            if totalResponses > 0 {
                percentage := float64(count) * 100 / float64(totalResponses)
                
                if i == correctAnswerID {
                    result.WriteString(fmt.Sprintf("✅ **%s**: %d (%.1f%%)\n", opt, count, percentage))
                    qStats.correct = count
                    qStats.correctUsers = optionUsers[i]
                    
                    if !isAnon {
                        // Update user stats
                        for _, userID := range optionUsers[i] {
                            if _, exists := userStats[userID]; !exists {
                                userStats[userID] = &UserStats{}
                            }
                            userStats[userID].correct++
                        }
                    }
                }
                
                // result.WriteString(fmt.Sprintf("**%s**: %d (%.1f%%)\n", opt, count, percentage))
                
                // Update total stats for this question
                qStats.total += count
                
                if !isAnon {
                    // Update user totals
                    for _, userID := range optionUsers[i] {
                        if _, exists := userStats[userID]; !exists {
                            userStats[userID] = &UserStats{}
                        }
                        userStats[userID].total++
                    }
                }
            }
        }
        
        if totalResponses == 0 {
            result.WriteString("❌ *No responses*\n")
        }

        if correctAnswerID >= 0 && totalResponses > 0 {
            correctCount := optionCounts[correctAnswerID]
            // correctPercentage := float64(correctCount) * 100 / float64(totalResponses)
            // result.WriteString(fmt.Sprintf("\nCorrect answers: %d (%.1f%%)\n", correctCount, correctPercentage))
            
            // Update global stats
            totalCorrect += correctCount
            totalAnswers += totalResponses
        }

        questionStats = append(questionStats, qStats)
        // result.WriteString("\n---\n")
        result.WriteString("\n")
    }

    if len(questionStats) == 0 {
        b.respondError(e, "❌ *No question/response data!*")
        return err
    }

    // Generate summary
    result.WriteString("### \n**Summary**\n\n")
    
    // Top 10 users
    type UserRank struct {
        id      string
        correct int
        total   int
    }
    var userRanks []UserRank
    for userID, stats := range userStats {
        userRanks = append(userRanks, UserRank{userID, stats.correct, stats.total})
    }
    sort.Slice(userRanks, func(i, j int) bool {
        return userRanks[i].correct > userRanks[j].correct
    })

    result.WriteString("**Top 10 Users**\n")
    if anyAnon {
        result.WriteString("-# ㊙️ *Anonymous answers are not counted*\n")
    }
    for i := 0; i < len(userRanks) && i < 10; i++ {
        user := userRanks[i]
        if user.total > 0 {
            percentage := float64(user.correct) * 100 / float64(user.total)
            result.WriteString(fmt.Sprintf("%d. <@%s>: %d/%d correct (%.1f%%)\n", 
                i+1, user.id, user.correct, user.total, percentage))
        }
    }
    result.WriteString("\n")

    // Questions ranked by correct answers
    sort.Slice(questionStats, func(i, j int) bool {
        iPerc := float64(0)
        if questionStats[i].total > 0 {
            iPerc = float64(questionStats[i].correct) / float64(questionStats[i].total)
        }
        jPerc := float64(0)
        if questionStats[j].total > 0 {
            jPerc = float64(questionStats[j].correct) / float64(questionStats[j].total)
        }
        return iPerc > jPerc
    })

    result.WriteString("**Questions Ranked by Correct Answers**\n")
    for i, q := range questionStats {
        if q.total > 0 {
            percentage := float64(q.correct) * 100 / float64(q.total)
            result.WriteString(fmt.Sprintf("%d. Q#%d: %.1f%% correct (%d/%d)\n",
                i+1, q.id, percentage, q.correct, q.total))
        }
    }
    result.WriteString("\n")

    // Overall statistics
    if totalAnswers > 0 {
        overallPercentage := float64(totalCorrect) * 100 / float64(totalAnswers)
        result.WriteString(fmt.Sprintf("**Overall Statistics**\n"))
        result.WriteString(fmt.Sprintf("Total answers: %d\n", totalAnswers))
        result.WriteString(fmt.Sprintf("Correct answers: %d (%.1f%%)\n", totalCorrect, overallPercentage))
    }

    if showToEveryone {
        b.respond(e, result.String(), 0)
    } else {
        b.respond(e, result.String(), discord.EphemeralMessage)
    }

    return nil
}