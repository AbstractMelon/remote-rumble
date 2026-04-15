package models

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email,omitempty"`
	IsGuest   bool   `json:"isGuest"`
	IsAdmin   bool   `json:"isAdmin"`
	CreatedAt int64  `json:"createdAt"`
}

type Bot struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	Online      bool   `json:"online"`
	CreatedAt   int64  `json:"createdAt"`
}

type QueueEntry struct {
	UserID    int64  `json:"userId"`
	Username  string `json:"username"`
	Position  int    `json:"pos"`
	JoinedAt  int64  `json:"joinedAt"`
	IsCurrent bool   `json:"isCurrent,omitempty"`
}

type Fight struct {
	ID          int64  `json:"id"`
	Player1ID   int64  `json:"player1Id"`
	Player2ID   int64  `json:"player2Id"`
	Player1Name string `json:"player1Name,omitempty"`
	Player2Name string `json:"player2Name,omitempty"`
	Bot1ID      string `json:"bot1Id,omitempty"`
	Bot2ID      string `json:"bot2Id,omitempty"`
	StartedAt   int64  `json:"startedAt,omitempty"`
	EndedAt     int64  `json:"endedAt,omitempty"`
	WinnerID    int64  `json:"winnerId,omitempty"`
	Status      string `json:"status"`
}
