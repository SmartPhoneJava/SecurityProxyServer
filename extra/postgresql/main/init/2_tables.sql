\c proxybase

/*
	Method       string
	RemoteAddr   string
	Body         string
	Header       map[string]string
	UserLogin    string
	UserPassword string
*/

CREATE TABLE Request (
  id SERIAL PRIMARY KEY,
  method text NOT NULL,
  scheme text NOT NULL,
	address text NOT NULL,
	header text default '',
	body text default '',
	userLogin text default '',
	userPassword text default '',
	add TIMESTAMPTZ default now()
);