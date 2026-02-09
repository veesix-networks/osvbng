package isis

type Area struct {
	Area     string    `json:"area"`
	Circuits []Circuit `json:"circuits"`
}

type Circuit struct {
	Circuit   int    `json:"circuit"`
	Adj       string `json:"adj,omitempty"`
	Interface string `json:"interface,omitempty"`
	Level     int    `json:"level,omitempty"`
	State     string `json:"state,omitempty"`
	ExpiresIn string `json:"expires-in,omitempty"`
	SNPA      string `json:"snpa,omitempty"`
}
