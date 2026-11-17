// Command dfctl is the operator CLI: it works directly on the database.
//
//	dfctl migrate
//	dfctl admin create --email you@example.com --password '…' [--name Name] [--role owner|admin|viewer]
//	dfctl admin reset-password --email you@example.com --password '…'
//	dfctl apikey create --name ci [--test] [--email owner@example.com]
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"

	"fulfillment/internal/auth"
	"fulfillment/internal/config"
	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	crypt.SetSecret(cfg.AppSecret)
	g, err := db.Open(cfg.DatabaseURL, false)
	if err != nil {
		fatal(err)
	}
	if err := db.Migrate(g); err != nil {
		fatal(err)
	}
	if _, err := db.EnsureDefaultStore(g, cfg.PublicURL); err != nil {
		fatal(err)
	}

	switch os.Args[1] {
	case "migrate":
		fmt.Println("database is up to date")
	case "admin":
		if len(os.Args) < 3 {
			usage()
		}
		adminCmd(g, os.Args[2], os.Args[3:])
	case "apikey":
		if len(os.Args) < 3 || os.Args[2] != "create" {
			usage()
		}
		apikeyCreate(g, os.Args[3:])
	default:
		usage()
	}
}

func adminCmd(g *gorm.DB, sub string, args []string) {
	fs := flag.NewFlagSet("admin "+sub, flag.ExitOnError)
	email := fs.String("email", "", "email address")
	password := fs.String("password", "", "password (min 10 characters)")
	name := fs.String("name", "Admin", "display name")
	role := fs.String("role", auth.RoleOwner, "owner | admin | viewer")
	_ = fs.Parse(args)
	if *email == "" || len(*password) < 10 {
		fatal(fmt.Errorf("--email and a --password of at least 10 characters are required"))
	}
	hash, err := auth.HashPassword(*password)
	if err != nil {
		fatal(err)
	}
	switch sub {
	case "create":
		if !auth.ValidRole(*role) {
			fatal(fmt.Errorf("invalid role %q", *role))
		}
		u := db.User{Email: strings.TrimSpace(*email), Name: *name, PasswordHash: hash, Role: *role, CreatedAt: db.Now(), UpdatedAt: db.Now()}
		if err := g.Create(&u).Error; err != nil {
			fatal(err)
		}
		fmt.Printf("created %s %s (id %d)\n", u.Role, u.Email, u.ID)
	case "reset-password":
		res := g.Model(&db.User{}).Where("LOWER(email) = ?", strings.ToLower(*email)).Updates(map[string]any{"password_hash": hash, "updated_at": db.Now()})
		if res.Error != nil {
			fatal(res.Error)
		}
		if res.RowsAffected == 0 {
			fatal(fmt.Errorf("no user with email %s", *email))
		}
		g.Where("user_id IN (?)", g.Model(&db.User{}).Select("id").Where("LOWER(email) = ?", strings.ToLower(*email))).Delete(&db.AdminSession{})
		fmt.Println("password updated; existing sessions were revoked")
	default:
		usage()
	}
}

func apikeyCreate(g *gorm.DB, args []string) {
	fs := flag.NewFlagSet("apikey create", flag.ExitOnError)
	name := fs.String("name", "cli", "key name")
	test := fs.Bool("test", false, "create a test-mode key")
	email := fs.String("email", "", "owner email (default: first owner)")
	_ = fs.Parse(args)
	var u db.User
	q := g.Order("id")
	if *email != "" {
		q = q.Where("LOWER(email) = ?", strings.ToLower(*email))
	} else {
		q = q.Where("role = ?", auth.RoleOwner)
	}
	if err := q.First(&u).Error; err != nil {
		fatal(fmt.Errorf("user not found (create one with: dfctl admin create)"))
	}
	_, token, err := auth.NewAPIKey(g, u.ID, *name, *test, nil)
	if err != nil {
		fatal(err)
	}
	fmt.Println(token)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dfctl migrate | admin create|reset-password --email … --password … | apikey create --name … [--test]")
	os.Exit(2)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
