package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDomainIndex_ExactMatch(t *testing.T) {
	idx := newDomainIndex()
	a := &acmeEntry{configID: 1}
	idx.insert("api.foo.com", a)

	assert.Same(t, a, idx.lookup("api.foo.com"))
	assert.Nil(t, idx.lookup("foo.com"))
	assert.Nil(t, idx.lookup("other.foo.com"))
}

func TestDomainIndex_WildcardMatchesDescendantsAtAnyDepth(t *testing.T) {
	idx := newDomainIndex()
	b := &acmeEntry{configID: 1}
	idx.insert("*.foo.com", b)

	assert.Same(t, b, idx.lookup("api.foo.com"))
	assert.Same(t, b, idx.lookup("v1.api.foo.com"))
	assert.Same(t, b, idx.lookup("deeply.nested.path.foo.com"))
}

func TestDomainIndex_WildcardDoesNotMatchParent(t *testing.T) {
	idx := newDomainIndex()
	b := &acmeEntry{configID: 1}
	idx.insert("*.foo.com", b)

	assert.Nil(t, idx.lookup("foo.com"), "*.foo.com must not match foo.com")
}

func TestDomainIndex_ExactBeatsWildcard(t *testing.T) {
	idx := newDomainIndex()
	wildcard := &acmeEntry{configID: 1}
	exact := &acmeEntry{configID: 2}
	idx.insert("*.foo.com", wildcard)
	idx.insert("api.foo.com", exact)

	assert.Same(t, exact, idx.lookup("api.foo.com"))
	assert.Same(t, wildcard, idx.lookup("other.foo.com"))
}

func TestDomainIndex_DeeperWildcardBeatsShallowerWildcard(t *testing.T) {
	idx := newDomainIndex()
	shallow := &acmeEntry{configID: 1}
	deep := &acmeEntry{configID: 2}
	idx.insert("*.foo.com", shallow)
	idx.insert("*.api.foo.com", deep)

	assert.Same(t, deep, idx.lookup("v1.api.foo.com"))
	assert.Same(t, deep, idx.lookup("x.y.api.foo.com"))
	assert.Same(t, shallow, idx.lookup("api.foo.com"), "the deep wildcard only matches strict descendants of api.foo.com")
	assert.Same(t, shallow, idx.lookup("other.foo.com"))
}

func TestDomainIndex_ConflictReturnsPreviousOwner(t *testing.T) {
	idx := newDomainIndex()
	a := &acmeEntry{configID: 1}
	b := &acmeEntry{configID: 2}

	assert.Nil(t, idx.insert("api.foo.com", a))
	prev := idx.insert("api.foo.com", b)
	assert.Same(t, a, prev, "re-inserting an existing key should return the previous owner")
	assert.Same(t, b, idx.lookup("api.foo.com"), "last writer wins")
}

func TestDomainIndex_CaseAndTrailingDotNormalised(t *testing.T) {
	idx := newDomainIndex()
	a := &acmeEntry{configID: 1}
	idx.insert("API.Foo.Com.", a)

	assert.Same(t, a, idx.lookup("api.foo.com"))
	assert.Same(t, a, idx.lookup("API.FOO.COM"))
	assert.Same(t, a, idx.lookup("api.foo.com."))
}

func TestDomainIndex_UnrelatedNameReturnsNil(t *testing.T) {
	idx := newDomainIndex()
	idx.insert("*.foo.com", &acmeEntry{configID: 1})

	assert.Nil(t, idx.lookup("bar.org"))
	assert.Nil(t, idx.lookup(""))
	assert.Nil(t, idx.lookup("com"))
}

func TestDomainIndex_BareWildcardRejected(t *testing.T) {
	idx := newDomainIndex()
	a := &acmeEntry{configID: 1}

	assert.Nil(t, idx.insert("*", a), "bare * should be skipped (would match the entire DNS namespace)")
	assert.Nil(t, idx.lookup("anything.com"))
}

func TestDomainIndex_MixedExactAndWildcardSiblings(t *testing.T) {
	idx := newDomainIndex()
	apexExact := &acmeEntry{configID: 1}
	subWildcard := &acmeEntry{configID: 2}
	idx.insert("foo.com", apexExact)
	idx.insert("*.foo.com", subWildcard)

	assert.Same(t, apexExact, idx.lookup("foo.com"))
	assert.Same(t, subWildcard, idx.lookup("anything.foo.com"))
	assert.Same(t, subWildcard, idx.lookup("deep.path.foo.com"))
}
