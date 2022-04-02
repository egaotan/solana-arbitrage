package config

type Node struct {
	Rpc    string `json:"rpc"`
	Ws     string `json:"ws"`
	Usable bool   `json:"usable"`
}

type Config struct {
	SlotNodes []*Node `json:"slot_nodes"`
	LssNodes  []*Node `json:"lss_nodes"`
	Bomb      int     `json:"bomb"`
}
