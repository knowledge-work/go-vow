// Package readonlyWrites pins the readonly check: a field assignment on a
// marked type is reported once the instance has left the function that
// built it, and is silent before that. The escape shapes here are the ones
// the nullness passes already recognise — return, store into a global,
// channel send, and a call the value is handed to as an argument.
package readonlyWrites

// Config is the marked type. Every fixture below builds one and then
// assigns a field, differing only in whether the value escaped first.
//
// vow:readonly
type Config struct {
	Name  string
	Rules []string
}

var shared *Config

// fillsBeforeReturning is the construction window: nothing has let the
// value out when the fields are written, so the marker is satisfied.
func fillsBeforeReturning(name string) *Config {
	c := &Config{}
	c.Name = name
	c.Rules = []string{"a"}
	return c
}

// writesAfterStoringGlobally hands the value to a package variable and
// then keeps writing.
func writesAfterStoringGlobally(name string) {
	c := &Config{}
	shared = c
	c.Name = name // want `vow\[readonly\]: Config reassigns a field after the value escaped`
}

// writesAfterPassingToCall hands the value to a function that could hold
// on to it.
func writesAfterPassingToCall(name string) {
	c := &Config{}
	consume(c)
	c.Name = name // want `vow\[readonly\]: Config reassigns a field after the value escaped`
}

// writesAfterSending hands the value to a channel.
func writesAfterSending(ch chan *Config, name string) {
	c := &Config{}
	ch <- c
	c.Name = name // want `vow\[readonly\]: Config reassigns a field after the value escaped`
}

// writesThroughTheReferenceStaysSilent pins the Java-final boundary: the
// field is not reassigned, so replacing an element is outside the marker
// even though the value has escaped.
func writesThroughTheReferenceStaysSilent() {
	c := &Config{Rules: []string{"a"}}
	consume(c)
	c.Rules[0] = "b"
}

// readOnlyMethodKeepsTheWindowOpen pins the carve-out: calling a method on
// the value under construction is not an escape, so the field assignment
// after it is still inside the window.
func readOnlyMethodKeepsTheWindowOpen(name string) *Config {
	c := &Config{}
	c.validate()
	c.Name = name
	return c
}

// copyOfAParameterIsItsOwnInstance pins the other carve-out: the callee
// holds a copy, so assigning its field cannot reach the caller.
func copyOfAParameterIsItsOwnInstance(c Config, name string) Config {
	c.Name = name
	return c
}

func (c *Config) validate() bool { return c.Name != "" }

// passesAsMethodExpression spells the same call as readOnlyMethodKeepsThe-
// WindowOpen through a method expression, which compiles to a thunk taking
// the receiver as its first argument. Both spellings leave the window open.
func passesAsMethodExpression(name string) *Config {
	c := &Config{}
	_ = (*Config).validate(c)
	c.Name = name
	return c
}

func (c *Config) merge(*Config) {}

// passesToABoundMethod pins the other side of that: a bound method value
// carries the receiver in the closure, so the argument is an argument and
// handing the value over lets it out.
func passesToABoundMethod(name string) {
	c := &Config{}
	other := &Config{}
	mergeInto := c.merge
	mergeInto(other)
	other.Name = name // want `vow\[readonly\]: Config reassigns a field after the value escaped`
}

type validator interface{ validate() bool }

// interfaceMethodKeepsTheWindowOpen reaches the same answer as the carve-out
// above by a different route. The call is in invoke mode, where the receiver
// sits in Common().Value rather than in Args, so the operand scan that skips
// a receiver in Args never sees it.
func interfaceMethodKeepsTheWindowOpen(name string) *Config {
	c := &Config{}
	var v validator = c
	_ = v.validate()
	c.Name = name
	return c
}

type sink interface{ take(*Config) }

type discardingSink struct{}

func (discardingSink) take(*Config) {}

// passesToAnInterfaceMethodArgument hands the value to an interface method
// as its first true argument. The receiver of an invoke-mode call is not in
// Args, so Args[0] holds the value itself and the call lets it out.
func passesToAnInterfaceMethodArgument(name string) {
	var s sink = discardingSink{}
	c := &Config{}
	s.take(c)
	c.Name = name // want `vow\[readonly\]: Config reassigns a field after the value escaped`
}

func consume(*Config) {}

// escapingMethodIsNotSeen records what the receiver-position carve-out
// gives up. A method can store its receiver, and the check does not read
// the callee, so the assignment below stays silent even though the value
// left the function. Closing this would mean counting every method call as
// an escape, which shuts the window on the first validate().
func escapingMethodIsNotSeen(name string) {
	c := &Config{}
	c.register()
	c.Name = name
}

func (c *Config) register() { shared = c }

// Weight is marked but is not a struct, so no field address can name it and
// nothing here can report. It pins that a non-struct marker is collected
// without reaching the walk.
//
// vow:readonly
type Weight int

// Plain is the type the alias below names. It carries no marker of its
// own, so the alias is the only thing that could register it.
type Plain struct {
	Name string
}

var plainShared *Plain

// PlainAlias carries the marker but is an alias, whose type is an
// *types.Alias rather than a named type, so the collection skips it and
// Plain stays unregistered.
//
// vow:readonly
type PlainAlias = Plain

// writesToTheTypeAnAliasMarked names Plain rather than the alias, so the
// silence below rests only on the marker not registering: were it to
// register, this write after the escape would report.
func writesToTheTypeAnAliasMarked(name string) {
	p := &Plain{}
	plainShared = p
	p.Name = name
}

// The marker on the group reaches both specs, the same way a `vow:define`
// on a type group registers every spec inside it.
//
// vow:readonly
type (
	GroupedFirst struct {
		Name string
	}
	GroupedSecond struct {
		Name string
	}
)

func writesGroupedFirstAfterEscape(name string) *GroupedFirst {
	g := &GroupedFirst{}
	consumeGroupedFirst(g)
	g.Name = name // want `vow\[readonly\]: GroupedFirst reassigns a field after the value escaped`
	return g
}

func writesGroupedSecondAfterEscape(name string) *GroupedSecond {
	g := &GroupedSecond{}
	consumeGroupedSecond(g)
	g.Name = name // want `vow\[readonly\]: GroupedSecond reassigns a field after the value escaped`
	return g
}

type (
	// MarkedInGroup carries the marker on its own spec, so it does not
	// reach the spec beside it.
	//
	// vow:readonly
	MarkedInGroup struct {
		Name string
	}

	UnmarkedInGroup struct {
		Name string
	}
)

func writesMarkedInGroupAfterEscape(name string) *MarkedInGroup {
	m := &MarkedInGroup{}
	consumeMarkedInGroup(m)
	m.Name = name // want `vow\[readonly\]: MarkedInGroup reassigns a field after the value escaped`
	return m
}

func writesUnmarkedInGroupAfterEscape(name string) *UnmarkedInGroup {
	u := &UnmarkedInGroup{}
	consumeUnmarkedInGroup(u)
	u.Name = name
	return u
}

func consumeGroupedFirst(*GroupedFirst) {}

func consumeGroupedSecond(*GroupedSecond) {}

func consumeMarkedInGroup(*MarkedInGroup) {}

func consumeUnmarkedInGroup(*UnmarkedInGroup) {}

// boxingIntoAnInterface hands the value to an interface parameter, which
// it reaches through a MakeInterface. The walk follows that instruction, so
// the boxed value closes the window the way a typed argument does.
func boxingIntoAnInterface(name string) {
	c := &Config{}
	consumeAnything(c)
	c.Name = name // want `vow\[readonly\]: Config reassigns a field after the value escaped`
}

func consumeAnything(any) {}
