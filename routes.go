package nsync

import "github.com/tedsuo/rata"

const (
	DesireAppRoute = "Desire"
	StopAppRoute   = "StopApp"
	KillIndexRoute = "KillIndex"
)

var Routes = rata.Routes{
	{Path: "/v1/apps/:process_guid", Method: "PUT", Name: DesireAppRoute},
	{Path: "/v1/apps/:process_guid", Method: "DELETE", Name: StopAppRoute},
	{Path: "/v1/apps/:process_guid/index/:index", Method: "DELETE", Name: KillIndexRoute},
}
