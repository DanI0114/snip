package main

import (
	"log"
	"net/http"
	"time"
)

type myLinkResponse struct {
	Code      string    `json:"code"`
	TargetURL string    `json:"target_url"`
	ShortURL  string    `json:"short_url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

func (app *application) myLinks(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(r)

	if !ok {
		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "authenticated user missing",
			},
		)
		return
	}

	rows, err := app.db.QueryContext(
		r.Context(),
		`
			SELECT
				code,
				target_url,
				clicks,
				created_at
			FROM short_links
			WHERE user_id = $1
			ORDER BY created_at DESC
		`,
		userID,
	)

	if err != nil {
		log.Printf(
			"load links for user %d: %v",
			userID,
			err,
		)

		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "could not load links",
			},
		)
		return
	}

	defer rows.Close()

	links := make([]myLinkResponse, 0)

	for rows.Next() {
		var link myLinkResponse

		err := rows.Scan(
			&link.Code,
			&link.TargetURL,
			&link.Clicks,
			&link.CreatedAt,
		)

		if err != nil {
			log.Printf("scan user link: %v", err)

			writeJSON(
				w,
				http.StatusInternalServerError,
				errorResponse{
					Error: "could not load links",
				},
			)
			return
		}

		link.ShortURL =
			app.baseURL + "/" + link.Code

		links = append(links, link)
	}

	if err := rows.Err(); err != nil {
		log.Printf("iterate user links: %v", err)

		writeJSON(
			w,
			http.StatusInternalServerError,
			errorResponse{
				Error: "could not load links",
			},
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"links": links,
		},
	)
}
