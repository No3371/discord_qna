package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

type Bot struct {
    s     *state.State
    db    *sql.DB
    token string
}

func main() {
    err := godotenv.Load()
    if err != nil {
        panic(err)
    }
    token := os.Getenv("BOT_TOKEN")
    if token == "" {
        log.Fatal("BOT_TOKEN environment variable is required")
    }

    bot := &Bot{token: token}
    if err := bot.Start(); err != nil {
        log.Fatal("Failed to start bot:", err)
    }
}

func (b *Bot) Start() error {
    // Initialize SQLite
    db, err := sql.Open("sqlite", "poll.db")
    if err != nil {
        return fmt.Errorf("failed to open database: %w", err)
    }
    b.db = db
    defer b.db.Close()

    // Create tables
    if err := b.createTables(); err != nil {
        return fmt.Errorf("failed to create tables: %w", err)
    }

    // Initialize Discord client
    b.s = state.New("Bot " + b.token)
    b.s.AddHandler(b.handleInteraction)
    b.s.AddIntents(gateway.IntentGuilds | gateway.IntentGuildMessages)

	b.s.AddHandler(func(m *gateway.ReadyEvent) {
        if err := b.registerCommands(); err != nil {
            panic(fmt.Errorf("failed to register commands: %w", err))
        }
	})

    // Start the bot
    if err := b.s.Connect(context.Background()); err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }

    return nil
}

func (b *Bot) createTables() error {
    queries := []string{
        `CREATE TABLE IF NOT EXISTS questions (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            creator_id TEXT NOT NULL,
            guild_id TEXT NOT NULL,
            question TEXT NOT NULL,
            options TEXT NOT NULL,
            correct_answer_id INTEGER NOT NULL,
            creation_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            message_id TEXT NOT NULL,
            channel_id TEXT NOT NULL,
            is_closed BOOLEAN DEFAULT FALSE
        )`,
        `CREATE TABLE IF NOT EXISTS responses (
            question_id INTEGER,
            user_id TEXT NOT NULL,
            choice TEXT NOT NULL,
            response_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY(question_id) REFERENCES questions(id),
            UNIQUE(question_id, user_id)
        )`,
    }

    for _, query := range queries {
        if _, err := b.db.Exec(query); err != nil {
            return err
        }
    }
    return nil
}

func (b *Bot) registerCommands() error {
    perm := discord.PermissionManageChannels
    commands := []api.CreateCommandData{
        {
            Name:        "ask",
            Description: "Create a new poll",
            Options: []discord.CommandOption{
                &discord.StringOption{
                    OptionName:  "question",
                    Description: "The question to ask",
                    Required:    true,
                },
                &discord.IntegerOption{
                    OptionName:  "answer_id",
                    Description: "Which option is correct?",
                    Choices: []discord.IntegerChoice{
                        {Name: "opt1", Value: 0},
                        {Name: "opt2", Value: 1},
                        {Name: "opt3", Value: 2},
                        {Name: "opt4", Value: 3},
                        {Name: "opt5", Value: 4},
                        {Name: "opt6", Value: 5},
                    },
                    Required:    false,
                },
                &discord.StringOption{
                    OptionName:  "option1",
                    Description: "1st option",
                    Required:    false,
                },
                &discord.StringOption{
                    OptionName:  "option2",
                    Description: "2nd option",
                    Required:    false,
                },
                &discord.StringOption{
                    OptionName:  "option3",
                    Description: "3rd option",
                    Required:    false,
                },
                &discord.StringOption{
                    OptionName:  "option4",
                    Description: "4th option",
                    Required:    false,
                },
                &discord.StringOption{
                    OptionName:  "option5",
                    Description: "5th option",
                    Required:    false,
                },
                &discord.StringOption{
                    OptionName:  "option6",
                    Description: "6th option",
                    Required:    false,
                },
                // &discord.BooleanOption{
                //     OptionName:  "custom",
                //     Description: "Accept custom answer?",
                //     Required:    false,
                // },
                // Add more options up to option8...
            },
            DefaultMemberPermissions: &perm,
        },
        {
            Name:        "result",
            Description: "Show poll results",
            Options: []discord.CommandOption{
                &discord.IntegerOption{
                    OptionName:  "question_id",
                    Description: "ID of the poll",
                    Required:    true,
                },
                &discord.BooleanOption{
                    OptionName:  "public",
                    Description: "Show to everyone",
                    Required:    false,
                },
            },
            DefaultMemberPermissions: &perm,
        },
        {
            Name:        "close",
            Description: "Close a poll",
            Options: []discord.CommandOption{
                &discord.IntegerOption{
                    OptionName:  "question_id",
                    Description: "ID of the poll",
                    Required:    true,
                },
            },
            DefaultMemberPermissions: &perm,
        },
        {
            Name:        "analyze",
            Description: "Analyze multiple questions and show statistics",
            Options: []discord.CommandOption{
                &discord.StringOption{
                    OptionName:  "question_ids",
                    Description: "Comma-separated list of question IDs (e.g. 1,2,3)",
                    Required:    true,
                },
                &discord.BooleanOption{
                    OptionName:  "public",
                    Description: "Show to everyone",
                    Required:    false,
                },
            },
            DefaultMemberPermissions: &perm,
        },
    }

    if _, err := b.s.BulkOverwriteCommands(b.s.Ready().Application.ID, commands); err != nil {
        return fmt.Errorf("overwrite: %w", err)
    }

    existingCommands, err := b.s.Commands(b.s.Ready().Application.ID)
    if err == nil {
        for _, command := range existingCommands {
            if command.Name == "askmd" {
                if err := b.s.DeleteCommand(b.s.Ready().Application.ID, command.ID); err != nil {
                    return fmt.Errorf("delete: %w", err)
                }
            }
        }
    }
    return nil
}

func (b *Bot) handleInteraction(e *gateway.InteractionCreateEvent) {
    defer func () {
        err := recover()
        if err != nil {
            b.respondError(e, fmt.Sprintf("Unhandled error: %v", err))
        }
    } ()
    switch data := e.Data.(type) {
    case *discord.CommandInteraction:
        switch data.Name {
        case "ask":
            b.handleAskCommand(e)
        case "result":
            b.handleResultCommand(e)
        case "close":
            b.handleCloseCommand(e)
        case "askmd":
            b.handleAskMarkdownCommand(e)
        case "analyze":
            b.handleAnalyzeCommand(e)
        }
    case *discord.ButtonInteraction:
        b.handleButtonClick(e)
    }
}

func (b *Bot) handleAskCommand(e *gateway.InteractionCreateEvent) {
    data := e.Data.(*discord.CommandInteraction)
    
    // Check permissions
    member, err := b.s.Member(e.GuildID, e.Member.User.ID)
    if err != nil {
        b.respondError(e, "Failed to get member")
        return
    }
    perms, err := b.s.Permissions(e.ChannelID, member.User.ID)
    if err != nil || !perms.Has(discord.PermissionManageChannels) {
        b.respondError(e, "You need to have the Manage Channels permission to create polls")
        return
    }

    question := data.Options[0].String()
    options := make([]string, 0)
    var answerId = 0
    
    // Collect options
    for i := 1; i < len(data.Options); i++ {
        if data.Options[i].Name == "answer" {
            // customAnswer, _ = data.Options[i].BoolValue()
        } else if data.Options[i].Name == "answer_id" {
            aId, _ :=  data.Options[i].IntValue()
            answerId = int(aId)
        } else if data.Options[i].String() != "" {
            options = append(options, data.Options[i].String())
        }
    }

    // Create buttons
    components := make([]discord.Component, len(options))
    for i, opt := range options {
        components[i] = &discord.ButtonComponent{
            CustomID: discord.ComponentID(fmt.Sprintf("opt_%d", i)),
            Label:    opt,
            Style:    discord.PrimaryButtonStyle(),
        }
    }

    // Store in database
    result, err := b.db.Exec(
        "INSERT INTO questions (creator_id, question, correct_answer_id, options, message_id, channel_id, guild_id) VALUES (?, ?, ?, ?, ?, ?, ?)",
        e.Member.User.ID,
        question,
        answerId,
        strings.Join(options, "|"),
        "0",
        e.ChannelID.String(),
        e.GuildID.String(),
    )
    if err != nil {
        b.respondError(e, "Failed to store poll")
        return
    }

    questionID, _ := result.LastInsertId()

    // Send poll message
    msg, err := b.s.SendMessageComplex(e.ChannelID, api.SendMessageData{
        Content:    question,
        Components: discord.Components(components...),
    })
    if err != nil {
        b.respondError(e, "Failed to announce poll")
        return
    }

    _, err = b.db.Exec(
        "UPDATE questions SET message_id = ? WHERE id = ?",
        msg.ID.String(),
        questionID,
    )
    if err != nil {
        b.respondError(e, fmt.Sprintf("Failed to set message ID %d for poll %d", msg.ID, questionID))
        b.s.DeleteMessage(msg.ChannelID, msg.ID, "")
        return
    }

    b.respond(e, fmt.Sprintf("Poll created with ID: %d", questionID), discord.EphemeralMessage)
}

func (b *Bot) handleButtonClick(e *gateway.InteractionCreateEvent) {
    data := e.Data.(*discord.ButtonInteraction)
    
    // Extract question ID from message
    var questionID int64
    err := b.db.QueryRow("SELECT id FROM questions WHERE message_id = ? AND is_closed = FALSE", e.Message.ID.String()).Scan(&questionID)
    if err != nil {
        b.respondError(e, "Poll not found or closed")
        return
    }

    // Record response
    _, err = b.db.Exec(
        "INSERT OR REPLACE INTO responses (question_id, user_id, choice) VALUES (?, ?, ?)",
        questionID,
        e.Member.User.ID,
        strings.TrimPrefix(string(data.CustomID), "opt_"),
    )
    if err != nil {
        b.respondError(e, "Failed to record response")
        return
    }

    b.respond(e, "Your response has been recorded!", discord.EphemeralMessage)
}

func (b *Bot) handleResultCommand(e *gateway.InteractionCreateEvent) {
    data := e.Data.(*discord.CommandInteraction)
    questionID, err := data.Options[0].IntValue()
    if err != nil {
        b.respondError(e, "Invalid question ID")
        return
    }

    showToEveryone := false
    if data.Options.Find("public").Name != "" {
        showToEveryone, err = data.Options[1].BoolValue()
        if err != nil {
            b.respondError(e, "Invalid public value")
            return
        }
    }

    // Check permissions
    member, err := b.s.Member(e.GuildID, e.Member.User.ID)
    if err != nil {
        b.respondError(e, "Failed to get member")
        return
    }
    perms, err := b.s.Permissions(e.ChannelID, member.User.ID)
    if err != nil || !perms.Has(discord.PermissionManageChannels) {
        b.respondError(e, "You need to have the Manage Channels permission to create polls")
        return
    }


    // Get question info
    var creationTime time.Time
    var options string
    var question string
    var aId int = -1
    var guildId string
    err = b.db.QueryRow(
        "SELECT question, creation_time, options, correct_answer_id, guild_id FROM questions WHERE id = ?",
        questionID,
    ).Scan(&question, &creationTime, &options, &aId, &guildId)
    if err != nil {
        b.respondError(e, "Poll not found")
        return
    }

    if guildId != e.GuildID.String() {
        b.respondError(e, "Poll not found")
        return
    }

    // Get response counts
    rows, err := b.db.Query(`
        SELECT 
            choice,
            COUNT(*) as count,
            GROUP_CONCAT(user_id) as users,
            GROUP_CONCAT(unixepoch(response_time)) as response_times
        FROM responses 
        WHERE question_id = ?
        GROUP BY choice
        ORDER BY MIN(response_time)`,
        questionID,
    )
    if err != nil {
        b.respondError(e, "Failed to get results")
        return
    }
    defer rows.Close()

    optionsList := strings.Split(options, "|")
    totalResponses := 0
    var result strings.Builder

    result.WriteString(fmt.Sprintf("**Question**\n%s\n\n", question))
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
        optionText := optionsList[choiceIdx]
        totalResponses += count
        
        if aId != -1 && choiceIdx == aId {
            result.WriteString("✅")
        }
        result.WriteString(fmt.Sprintf("**Option:** %s\n", optionText))
        result.WriteString(fmt.Sprintf("Responses: %d (%.1f%%)\n", count, float64(count)*100/float64(totalResponses)))
        for i, userID := range strings.Split(users, ",") {
            t := respTime[0]
            respTime = respTime[1:]
            result.WriteString(fmt.Sprintf("%d. <@%s> (<t:%d:R>)\n", i+1, userID, t.Unix()))
        }
    }

    result.WriteString(fmt.Sprintf("\nPoll created at: <t:%d> (<t:%d:R>)\n", creationTime.Unix(), creationTime.Unix()))
    result.WriteString(fmt.Sprintf("Total responses: %d", totalResponses))
    if showToEveryone {
        b.respond(e, result.String(), 0)
    } else {
        b.respond(e, result.String(), discord.EphemeralMessage)
    }
}

func (b *Bot) handleAskMarkdownCommand(e *gateway.InteractionCreateEvent) {
    data := e.Data.(*discord.CommandInteraction)
    
    // Check permissions
    member, err := b.s.Member(e.GuildID, e.Member.User.ID)
    if err != nil {
        b.respondError(e, "Failed to get member")
        return
    }
    perms, err := b.s.Permissions(e.ChannelID, member.User.ID)
    if err != nil || !perms.Has(discord.PermissionManageChannels) {
        b.respondError(e, "You need to have the Manage Channels permission to create polls")
        return
    }

    markdown := data.Options[0].String()
    lines := strings.Split(markdown, "\n")
    
    if len(lines) < 2 {
        b.respondError(e, "Invalid markdown format. Expected: [question]\\n- [option1]\\n- [option2]...")
        return
    }

    // Extract question and options
    question := strings.TrimSpace(lines[0])
    options := make([]string, 0)
    
    for _, line := range lines[1:] {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "-") {
            option := strings.TrimSpace(strings.TrimPrefix(line, "-"))
            if option != "" {
                options = append(options, option)
            }
        }
    }

    if len(options) == 0 {
        b.respondError(e, "No options found in markdown")
        return
    }

    // Create buttons
    components := make([]discord.Component, len(options))
    for i, opt := range options {
        components[i] = &discord.ButtonComponent{
            CustomID: discord.ComponentID(fmt.Sprintf("opt_%d", i)),
            Label:    opt,
            Style:    discord.PrimaryButtonStyle(),
        }
    }

    // Send poll message
    msg, err := b.s.SendMessageComplex(e.ChannelID, api.SendMessageData{
        Content:    question,
        Components: discord.Components(components...),
    })
    if err != nil {
        b.respondError(e, "Failed to create poll")
        return
    }

    // Store in database
    result, err := b.db.Exec(
        "INSERT INTO questions (creator_id, question, options, take_custom_answer, message_id, channel_id) VALUES (?, ?, ?, ?, ?, ?)",
        e.Member.User.ID,
        question,
        strings.Join(options, "|"),
        false,
        msg.ID.String(),
        e.ChannelID.String(),
    )
    if err != nil {
        b.respondError(e, "Failed to store poll")
        return
    }

    questionID, _ := result.LastInsertId()
    b.respond(e, fmt.Sprintf("Poll created with ID: %d", questionID), discord.EphemeralMessage)
}

func (b *Bot) handleAnalyzeCommand(e *gateway.InteractionCreateEvent) {
    data := e.Data.(*discord.CommandInteraction)
    
    // Check permissions
    member, err := b.s.Member(e.GuildID, e.Member.User.ID)
    if err != nil {
        b.respondError(e, "Failed to get member")
        return
    }
    perms, err := b.s.Permissions(e.ChannelID, member.User.ID)
    if err != nil || !perms.Has(discord.PermissionManageChannels) {
        b.respondError(e, "You need to have the Manage Channels permission to view analysis")
        return
    }

    showToEveryone := false
    if data.Options.Find("public").Name != "" {
        showToEveryone, err = data.Options[1].BoolValue()
        if err != nil {
            b.respondError(e, "Invalid public value")
            return
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
        err = b.db.QueryRow(
            "SELECT question, options, correct_answer_id, guild_id FROM questions WHERE id = ?",
            qID,
        ).Scan(&question, &options, &correctAnswerID, &guildId)
        if err != nil {
            continue
        }

        if guildId != e.GuildID.String() {
            b.respondError(e, fmt.Sprintf("Q#%d is not your poll!", qID))
            return
        }

        result.WriteString(fmt.Sprintf("\n**Question %d**: %s\n", qID, question))
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
                    
                    // Update user stats
                    for _, userID := range optionUsers[i] {
                        if _, exists := userStats[userID]; !exists {
                            userStats[userID] = &UserStats{}
                        }
                        userStats[userID].correct++
                    }
                }
                
                // result.WriteString(fmt.Sprintf("**%s**: %d (%.1f%%)\n", opt, count, percentage))
                
                // Update total stats for this question
                qStats.total += count
                
                // Update user totals
                for _, userID := range optionUsers[i] {
                    if _, exists := userStats[userID]; !exists {
                        userStats[userID] = &UserStats{}
                    }
                    userStats[userID].total++
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
        return
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
            result.WriteString(fmt.Sprintf("%d. Question %d: %.1f%% correct (%d/%d)\n",
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
}

func (b *Bot) handleCloseCommand(e *gateway.InteractionCreateEvent) {
    data := e.Data.(*discord.CommandInteraction)
    questionID, err := data.Options[0].IntValue()
    if err != nil {
        b.respondError(e, "Invalid question ID")
        return
    }

    // Check permissions
    member, err := b.s.Member(e.GuildID, e.Member.User.ID)
    if err != nil {
        b.respondError(e, "Failed to get member")
        return
    }
    perms, err := b.s.Permissions(e.ChannelID, member.User.ID)
    if err != nil || !perms.Has(discord.PermissionManageChannels) {
        b.respondError(e, "You need to have the Manage Channels permission to create polls")
        return
    }

    // Close the poll
    _, err = b.db.Exec("UPDATE questions SET is_closed = TRUE WHERE id = ? AND guild_id = ?", questionID, e.GuildID.String())
    if err != nil {
        b.respondError(e, "Failed to close poll")
        return
    }

    b.respond(e, fmt.Sprintf("Poll %d has been closed", questionID), discord.EphemeralMessage)
}

func (b *Bot) respond(e *gateway.InteractionCreateEvent, content string, flags discord.MessageFlags) {
    err := b.s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
        Type: api.MessageInteractionWithSource,
        Data: &api.InteractionResponseData{
            Content: option.NewNullableString(content),
            Flags: flags,
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
            Content: option.NewNullableString("Error: " + message),
            Flags:   discord.EphemeralMessage,
        },
    })
    if err != nil {
        log.Printf("Failed to respond to interaction: %v", err)
    }
}
