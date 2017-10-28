package room

import (
	"encoding/json"
	"log"
	"time"

	"hawx.me/code/retro/database"
	"hawx.me/code/retro/sock"
)

func registerHandlers(r *Room, mux *sock.Server) {
	mux.Auth(func(auth sock.MsgAuth) bool {
		return r.IsUser(auth.Username, auth.Token)
	})

	mux.OnConnect(func(conn *sock.Conn) {
		conn.Send("", "hello", helloData{
			HasGitHub:    true,
			HasOffice365: false,
		})
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

			cards, err := r.db.GetCards(conn.Name, column.Id)
			if err != nil {
				log.Println(err)
			}
			for _, card := range cards {
				conn.Send("", "card", cardData{column.Id, card.Id, card.Revealed, card.Votes, card.TotalVotes})

				contents, _ := r.db.GetContents(card.Id)
				for _, content := range contents {
					conn.Send(content.Author, "content", contentData{column.Id, card.Id, content.Id, content.Text})
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
			Revealed: false,
		}

		if err := r.db.AddCard(card); err != nil {
			log.Println("add db:", err)
			return
		}

		content := database.Content{
			Id:     strId(),
			Card:   card.Id,
			Text:   args.CardText,
			Author: conn.Name,
		}

		if err := r.db.AddContent(content); err != nil {
			log.Println("add db:", err)
			return
		}

		conn.Broadcast("", "card", cardData{args.ColumnId, card.Id, card.Revealed, card.Votes, card.TotalVotes})

		conn.Broadcast(content.Author, "content", contentData{args.ColumnId, content.Card, content.Id, content.Text})
	})

	mux.Handle("edit", func(conn *sock.Conn, data []byte) {
		var content contentData
		if err := json.Unmarshal(data, &content); err != nil {
			log.Println("add:", err)
			return
		}

		if err := r.db.UpdateContent(content.ContentId, content.CardText); err != nil {
			log.Println("update db:", err)
			return
		}

		conn.Broadcast(conn.Name, "content", content)
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

		args.UserId = conn.Name
		r.db.Vote(conn.Name, args.CardId)

		conn.Broadcast(conn.Name, "vote", args)
	})

	mux.Handle("unvote", func(conn *sock.Conn, data []byte) {
		var args voteData
		if err := json.Unmarshal(data, &args); err != nil {
			return
		}

		args.UserId = conn.Name
		r.db.Unvote(conn.Name, args.CardId)

		conn.Broadcast(conn.Name, "unvote", args)
	})

	mux.Handle("delete", func(conn *sock.Conn, data []byte) {
		var args deleteData
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

type msg struct {
	Id   string   `json:"id"`
	Op   string   `json:"op"`
	Args []string `json:"args"`
}

type errorData struct {
	Error string `json:"error"`
}

type helloData struct {
	HasGitHub    bool `json:"hasGitHub"`
	HasOffice365 bool `json:"hasOffice365"`
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
	ColumnId   string `json:"columnId"`
	CardId     string `json:"cardId"`
	Revealed   bool   `json:"revealed"`
	Votes      int    `json:"votes"`
	TotalVotes int    `json:"totalVotes"`
}

type contentData struct {
	ColumnId  string `json:"columnId"`
	CardId    string `json:"cardId"`
	ContentId string `json:"contentId"`
	CardText  string `json:"cardText"`
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
	UserId   string `json:"userId"`
	ColumnId string `json:"columnId"`
	CardId   string `json:"cardId"`
}

type deleteData struct {
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

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
