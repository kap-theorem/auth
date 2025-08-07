package models

type User struct {
	UserId string
	UserName string
	EmailId string
	Password string
	ClientId string
}

type Client struct {
	ClientId string
	ClientName string
	ClientSecret string
}

type Session struct {
	SessionId string
	UserId string
	RefreshToken string
	UserAgent string
	Expiration string
}

func GetAllModels() []interface{} {
	return []interface{}{
		&User{},
		&Client{},
		&Session{},
	}
}