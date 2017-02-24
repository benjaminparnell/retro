package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"hawx.me/code/retro/database"
	"hawx.me/code/retro/sock"
	"hawx.me/code/serve"

	"golang.org/x/oauth2"
)

func strId() string {
	id, _ := uuid.NewRandom()
	return id.String()
}

type msg struct {
	Id   string   `json:"id"`
	Op   string   `json:"op"`
	Args []string `json:"args"`
}

type errorData struct {
	Error string `json:"error"`
}

type stageData struct {
	Stage string `json:"stage"`
}

type columnData struct {
	ColumnId    string `json:"columnId"`
	ColumnName  string `json:"columnName"`
	ColumnOrder int    `json:"columnOrder"`
}

type cardData struct {
	ColumnId string `json:"columnId"`
	CardId   string `json:"cardId"`
	Revealed bool   `json:"revealed"`
	Votes    int    `json:"votes"`
}

type contentData struct {
	ColumnId string `json:"columnId"`
	CardId   string `json:"cardId"`
	CardText string `json:"cardText"`
}

type moveData struct {
	ColumnFrom string `json:"columnFrom"`
	ColumnTo   string `json:"columnTo"`
	CardId     string `json:"cardId"`
}

type revealData struct {
	ColumnId string `json:"columnId"`
	CardId   string `json:"cardId"`
}

type groupData struct {
	ColumnFrom string `json:"columnFrom"`
	CardFrom   string `json:"cardFrom"`
	ColumnTo   string `json:"columnTo"`
	CardTo     string `json:"cardTo"`
}

type voteData struct {
	ColumnId string `json:"columnId"`
	CardId   string `json:"cardId"`
}

type userData struct {
	Username string `json:"username"`
}

type retroData struct {
	Id           string    `json:"id"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"createdAt"`
	Participants []string  `json:"participants"`
}

type Room struct {
	server *sock.Server
	db     *database.Database

	mu    sync.RWMutex
	users map[string]string
}

func NewRoom(db *database.Database) *Room {
	room := &Room{
		db:     db,
		server: sock.NewServer(),
	}

	registerHandlers(room, room.server)

	return room
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (r *Room) AddUser(user, token string) {
	r.db.EnsureUser(database.User{
		Username: user,
		Token:    token,
	})
}

func (r *Room) IsUser(user, token string) bool {
	found, err := r.db.GetUser(user)

	return err == nil && found.Token == token
}

func registerHandlers(r *Room, mux *sock.Server) {
	mux.Auth(func(auth sock.MsgAuth) bool {
		return r.IsUser(auth.Username, auth.Token)
	})

	mux.Handle("joinRetro", func(conn *sock.Conn, data []byte) {
		var args struct {
			RetroId string
		}
		if err := json.Unmarshal(data, &args); err != nil {
			log.Println("joinRetro:", err)
			return
		}

		retro, err := r.db.GetRetro(args.RetroId)
		if err != nil {
			log.Println("joinRetro", args.RetroId, err)
			return
		}
		conn.RetroId = args.RetroId

		if retro.Stage != "" {
			conn.Send("", "stage", stageData{retro.Stage})
		}

		columns, err := r.db.GetColumns(args.RetroId)
		if err != nil {
			log.Println("columns", err)
			return
		}
		for _, column := range columns {
			conn.Send("", "column", columnData{column.Id, column.Name, column.Order})

			cards, _ := r.db.GetCards(column.Id)
			for _, card := range cards {
				conn.Send("", "card", cardData{column.Id, card.Id, card.Revealed, card.Votes})

				contents, _ := r.db.GetContents(card.Id)
				for _, content := range contents {
					conn.Send(content.Author, "content", contentData{column.Id, card.Id, content.Text})
				}
			}
		}
	})

	mux.Handle("menu", func(conn *sock.Conn, data []byte) {
		users, err := r.db.GetUsers()
		if err != nil {
			log.Println("users", err)
			return
		}
		for _, user := range users {
			conn.Send("", "user", userData{user.Username})
		}

		retros, err := r.db.GetRetros(conn.Name)
		if err != nil {
			log.Println("retros", err)
			return
		}
		for _, retro := range retros {
			participants, err := r.db.GetParticipants(retro.Id)
			if err != nil {
				log.Println("retros.participants", err)
				continue
			}

			conn.Send("", "retro", retroData{retro.Id, retro.Name, retro.CreatedAt, participants})
		}
	})

	mux.Handle("add", func(conn *sock.Conn, data []byte) {
		var args struct {
			ColumnId string
			CardText string
		}
		if err := json.Unmarshal(data, &args); err != nil {
			log.Println("add:", err)
			return
		}

		card := database.Card{
			Id:       strId(),
			Column:   args.ColumnId,
			Votes:    0,
			Revealed: false,
		}

		r.db.AddCard(card)

		content := database.Content{
			Id:     strId(),
			Card:   card.Id,
			Text:   args.CardText,
			Author: conn.Name,
		}

		r.db.AddContent(content)

		conn.Broadcast("", "card", cardData{args.ColumnId, card.Id, card.Revealed, card.Votes})

		conn.Broadcast(content.Author, "content", contentData{args.ColumnId, content.Card, content.Text})
	})

	mux.Handle("move", func(conn *sock.Conn, data []byte) {
		var args moveData
		if err := json.Unmarshal(data, &args); err != nil {
			return
		}

		r.db.MoveCard(args.CardId, args.ColumnTo)

		conn.Broadcast(conn.Name, "move", args)
	})

	mux.Handle("stage", func(conn *sock.Conn, data []byte) {
		var args stageData
		if err := json.Unmarshal(data, &args); err != nil {
			return
		}

		r.db.SetStage(conn.RetroId, args.Stage)

		conn.Broadcast(conn.Name, "stage", args)
	})

	mux.Handle("reveal", func(conn *sock.Conn, data []byte) {
		var args revealData
		if err := json.Unmarshal(data, &args); err != nil {
			return
		}

		r.db.RevealCard(args.CardId)

		conn.Broadcast(conn.Name, "reveal", args)
	})

	mux.Handle("group", func(conn *sock.Conn, data []byte) {
		var args groupData
		if err := json.Unmarshal(data, &args); err != nil {
			return
		}

		err := r.db.GroupCards(args.CardFrom, args.CardTo)
		if err != nil {
			log.Println(err)
		}

		conn.Broadcast(conn.Name, "group", args)
	})

	mux.Handle("vote", func(conn *sock.Conn, data []byte) {
		var args voteData
		if err := json.Unmarshal(data, &args); err != nil {
			return
		}

		r.db.VoteCard(args.CardId)

		conn.Broadcast(conn.Name, "vote", args)
	})

	mux.Handle("delete", func(conn *sock.Conn, data []byte) {
		var args voteData
		if err := json.Unmarshal(data, &args); err != nil {
			return
		}

		r.db.DeleteCard(args.CardId)

		conn.Broadcast(conn.Name, "delete", args)
	})

	mux.Handle("createRetro", func(conn *sock.Conn, data []byte) {
		var args struct {
			Name  string   `json:"name"`
			Users []string `json:"users"`
		}

		if err := json.Unmarshal(data, &args); err != nil {
			log.Println(err)
			return
		}

		retroId := strId()
		createdAt := time.Now()

		r.db.AddRetro(database.Retro{
			Id:        retroId,
			Name:      args.Name,
			Stage:     "",
			CreatedAt: createdAt,
		})

		r.db.AddColumn(database.Column{
			Id:    strId(),
			Retro: retroId,
			Name:  "Start",
			Order: 0,
		})

		r.db.AddColumn(database.Column{
			Id:    strId(),
			Retro: retroId,
			Name:  "More",
			Order: 1,
		})

		r.db.AddColumn(database.Column{
			Id:    strId(),
			Retro: retroId,
			Name:  "Keep",
			Order: 2,
		})

		r.db.AddColumn(database.Column{
			Id:    strId(),
			Retro: retroId,
			Name:  "Less",
			Order: 3,
		})

		r.db.AddColumn(database.Column{
			Id:    strId(),
			Retro: retroId,
			Name:  "Stop",
			Order: 4,
		})

		allParticipants := append(args.Users, conn.Name)

		for _, user := range allParticipants {
			r.db.AddParticipant(retroId, user)
		}

		conn.Send(conn.Name, "retro", retroData{retroId, args.Name, createdAt, allParticipants})
	})
}

func main() {
	var (
		clientID     = os.Getenv("GH_CLIENT_ID")
		clientSecret = os.Getenv("GH_CLIENT_SECRET")
		organisation = os.Getenv("ORGANISATION")

		port   = flag.String("port", "8080", "")
		socket = flag.String("socket", "", "")
		assets = flag.String("assets", "app/dist", "")
		dbPath = flag.String("db", "./db", "")
	)
	flag.Parse()

	db, err := database.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	room := NewRoom(db)

	http.Handle("/", http.FileServer(http.Dir(*assets)))
	http.Handle("/ws", room.server)

	ctx := context.Background()
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"user", "read:org"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
		},
	}

	http.HandleFunc("/oauth/login", func(w http.ResponseWriter, r *http.Request) {
		url := conf.AuthCodeURL("state", oauth2.AccessTypeOnline)

		http.Redirect(w, r, url, http.StatusFound)
	})

	http.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.FormValue("code")

		tok, err := conf.Exchange(ctx, code)
		if err != nil {
			log.Println(err)
			return
		}

		client := conf.Client(ctx, tok)

		user, err := getUser(client)
		if err != nil {
			log.Println(err)
			return
		}

		inOrg, err := isInOrg(client, organisation)
		if err != nil {
			log.Println(err)
			return
		}

		if inOrg {
			token := strId()
			room.AddUser(user, token)

			http.Redirect(w, r, "/?user="+user+"&token="+token, http.StatusFound)
		} else {
			http.Redirect(w, r, "/?error=not_in_org", http.StatusFound)
		}
	})

	serve.Serve(*port, *socket, http.DefaultServeMux)
}

func getUser(client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var data struct {
		Login string `json:"login"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	return data.Login, nil
}

func isInOrg(client *http.Client, expectedOrg string) (bool, error) {
	resp, err := client.Get("https://api.github.com/user/orgs")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var data []struct {
		Login string `json:"login"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return false, err
	}

	for _, org := range data {
		if org.Login == expectedOrg {
			return true, nil
		}
	}

	return false, nil
}
