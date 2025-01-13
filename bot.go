package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
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
            answer_id INTEGER NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            is_closed BOOLEAN DEFAULT FALSE
        )`,
        `CREATE TABLE IF NOT EXISTS responses (
            question_id INTEGER,
            user_id TEXT NOT NULL,
            choice INTEGER NOT NULL,
            responded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY(question_id) REFERENCES questions(id),
            PRIMARY KEY (question_id, user_id)
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
            Name:        "post",
            Description: "Post a question",
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
            Name:        "close",
            Description: "Close a poll",
            Options: []discord.CommandOption{
                &discord.StringOption{
                    OptionName:  "question_ids",
                    Description: "Comma-separated list of question IDs (e.g. 1,2,3)",
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
        {
            Name:        "list",
            Description: "Show a list of recent questions",
            Options: []discord.CommandOption {
                &discord.IntegerOption{
                    OptionName:  "count",
                    Description: "How many recent questions to show? <1-50>",
                    Required:    false,
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
            Type: discord.MessageCommand,
            Name:        "Make questions",
            Description: "",
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