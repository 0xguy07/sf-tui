package sf

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUserNotFound is returned by ResolveUser when nothing matches.
var ErrUserNotFound = errors.New("no user matches")

// ErrAmbiguousUser is returned when a display name matches several users.
type ErrAmbiguousUser struct {
	Ident      string
	Candidates []UserBrief
}

func (e *ErrAmbiguousUser) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d users match %q; pass the username or Id:", len(e.Candidates), e.Ident)
	for _, c := range e.Candidates {
		fmt.Fprintf(&b, "\n  %s  %s  %s", c.Name, c.Username, c.ID)
	}
	return b.String()
}

const userSOQL = "SELECT Id, Name, Username, IsActive, Profile.Name FROM User WHERE %s = '%s'"

// ResolveUser finds one user by Id (005…), username (contains @), or exact
// display name. Inactive users are included.
func ResolveUser(q SOQLRunner, org, ident string) (UserBrief, error) {
	ident = strings.TrimSpace(ident)
	col, val := "Name", ident
	switch {
	case (len(ident) == 15 || len(ident) == 18) && strings.HasPrefix(ident, "005"):
		col = "Id"
	case strings.Contains(ident, "@"):
		col, val = "Username", strings.ToLower(ident)
	}
	recs, err := queryRecords(q, org, fmt.Sprintf(userSOQL, col, soqlEscape(val)))
	if err != nil {
		return UserBrief{}, err
	}
	users := make([]UserBrief, 0, len(recs))
	for _, rec := range recs {
		u := UserBrief{
			ID:       stringField(rec, "Id"),
			Name:     stringField(rec, "Name"),
			Username: stringField(rec, "Username"),
			IsActive: boolField(rec, "IsActive"),
		}
		if p, ok := rec["Profile"].(map[string]any); ok {
			u.ProfileName = stringField(p, "Name")
		}
		users = append(users, u)
	}
	switch len(users) {
	case 0:
		return UserBrief{}, fmt.Errorf("%w %q", ErrUserNotFound, ident)
	case 1:
		return users[0], nil
	default:
		return UserBrief{}, &ErrAmbiguousUser{Ident: ident, Candidates: users}
	}
}
