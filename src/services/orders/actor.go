package orders

import "errors"

// Actor is who is asking. It is the only way a caller enters the Order
// module for anything that reads or writes an order: every query is
// resolved against the Actor, so there is no unscoped method a handler can
// forget to scope.
//
// The HTTP layer builds an Actor from the session and passes it down. It
// never passes a bare user id, and never decides ownership itself.
type Actor struct {
	// UserID is the account the order is filed under. Zero means a guest.
	UserID int

	// Admin is true for an account with the admin role. An administrator
	// sees and edits every order; nobody else does.
	Admin bool
}

// Guest is an unauthenticated caller. A guest may place an order but cannot
// read one back: guest orders are not addressable until they are linked to
// an account, so there is no id to leak.
func Guest() Actor { return Actor{} }

// Shopper is a signed-in customer acting on their own orders.
func Shopper(userID int) Actor { return Actor{UserID: userID} }

// Administrator is a signed-in account with the admin role, acting on any
// order.
func Administrator(userID int) Actor { return Actor{UserID: userID, Admin: true} }

// IsGuest reports whether the Actor is unauthenticated.
func (a Actor) IsGuest() bool { return a.UserID <= 0 }

// CanAdminister reports whether the Actor may act on any order. The Order
// module checks this itself so a route registered without WithAdmin still
// cannot administer orders.
func (a Actor) CanAdminister() bool { return a.Admin }

// ErrNotPermitted is returned when an Actor asks for something its role does
// not allow, such as a dashboard or a status transition.
var ErrNotPermitted = errors.New("not permitted")

// scope projects an Actor into the SQL-level predicate the repository uses.
// It is the one place an Actor becomes a WHERE clause.
func (a Actor) scope() Scope { return Scope{all: a.Admin, userID: a.UserID} }

// Scope is the SQL-level form of an Actor. Every repository read and write
// takes one, so the ownership check cannot be left out of a query.
type Scope struct {
	all    bool
	userID int
}

// AnyOrder matches every order, regardless of owner. It exists for
// repository-internal reads that are not caller-facing.
func AnyOrder() Scope { return Scope{all: true} }

// predicate renders the WHERE clause identifying one order within the
// scope, plus its arguments.
//
// A guest's scope is userID 0, and orders.user_id is either NULL or a real
// account id, so "user_id = 0" matches nothing. That is the intended
// behaviour: a guest is not an owner of anything.
func (s Scope) predicate(id int) (string, []any) {
	if s.all {
		return "id = ?", []any{id}
	}
	return "id = ? AND user_id = ?", []any{id, s.userID}
}

// clause renders a bare WHERE predicate for the scope, plus its arguments,
// for queries that do not narrow to a single order.
func (s Scope) clause() (string, []any) {
	if s.all {
		return "1=1", nil
	}
	return "user_id = ?", []any{s.userID}
}

// writeParams returns the arguments for a statement that writes columns of one
// order: the values being written first, then the predicate's arguments, in
// the order predicate put the placeholders. Building the list here rather than
// appending to the predicate's own arguments keeps the values in the right
// slots — appending would push them after the id and silently write the wrong
// row, or none at all.
func (s Scope) writeParams(id int, values ...any) []any {
	if s.all {
		return append(values, id)
	}
	return append(values, id, s.userID)
}
