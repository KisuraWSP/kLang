package runtime

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestIntervalWalkMaxScoresMatchesSamples(t *testing.T) {
	tests := []struct {
		values   []int
		edges    []intervalWalkEdge
		expected []int
	}{
		{
			values: []int{10, 20, 5},
			edges: []intervalWalkEdge{
				{u: 1, v: 2, high: 5, low: 2},
				{u: 2, v: 3, high: 4, low: 3},
			},
			expected: []int{25, 25, 24},
		},
		{
			values:   []int{100, 10},
			edges:    []intervalWalkEdge{{u: 1, v: 2, high: 10, low: 5}},
			expected: []int{110, 110},
		},
		{
			values:   []int{50, 50},
			expected: []int{-1, -1},
		},
		{
			values:   []int{114514},
			expected: []int{-1},
		},
		{
			values:   []int{1, 2, 3, 4},
			edges:    []intervalWalkEdge{{u: 3, v: 4, high: 2, low: 1}},
			expected: []int{-1, -1, 6, 6},
		},
		{
			values: []int{1, 4, 1},
			edges: []intervalWalkEdge{
				{u: 1, v: 3, high: 4, low: 1},
				{u: 1, v: 2, high: 2, low: 1},
			},
			expected: []int{6, 6, 6},
		},
		{
			values: []int{10, 3, 2},
			edges: []intervalWalkEdge{
				{u: 2, v: 3, high: 4, low: 4},
				{u: 2, v: 1, high: 3, low: 2},
			},
			expected: []int{13, 13, 7},
		},
		{
			values: []int{5, 2, 2},
			edges: []intervalWalkEdge{
				{u: 3, v: 2, high: 4, low: 4},
				{u: 1, v: 3, high: 5, low: 5},
			},
			expected: []int{10, 9, 10},
		},
		{
			values: []int{857147200, 381798978, 633421584, 956726892, 315899900},
			edges: []intervalWalkEdge{
				{u: 2, v: 1, high: 883474754, low: 795831571},
				{u: 2, v: 4, high: 657281748, low: 375466725},
				{u: 1, v: 3, high: 666641114, low: 444218918},
				{u: 2, v: 3, high: 901861650, low: 790895313},
				{u: 3, v: 2, high: 613790652, low: 96876004},
				{u: 2, v: 5, high: 852725279, low: 216601090},
				{u: 3, v: 4, high: 500240642, low: 193633892},
				{u: 2, v: 5, high: 210434355, low: 130646156},
				{u: 3, v: 2, high: 457018372, low: 279005896},
			},
			expected: []int{1740621954, 1740621954, 1740621954, 1614008640, 1709872479},
		},
	}

	for index, test := range tests {
		actual := intervalWalkMaxScores(test.values, test.edges)
		if !reflect.DeepEqual(actual, test.expected) {
			t.Fatalf("case %d: expected %v, got %v", index+1, test.expected, actual)
		}
	}
}

func TestIntervalWalkMaxScoresMatchesBruteForce(t *testing.T) {
	random := rand.New(rand.NewSource(2239))
	for iteration := 0; iteration < 500; iteration++ {
		vertexCount := random.Intn(5) + 1
		edgeCount := random.Intn(8)
		if vertexCount == 1 {
			edgeCount = 0
		}
		values := make([]int, vertexCount)
		for index := range values {
			values[index] = random.Intn(9) + 1
		}
		edges := make([]intervalWalkEdge, edgeCount)
		for index := range edges {
			left := random.Intn(vertexCount) + 1
			right := random.Intn(vertexCount) + 1
			for right == left {
				right = random.Intn(vertexCount) + 1
			}
			high := random.Intn(6) + 1
			edges[index] = intervalWalkEdge{
				u:    left,
				v:    right,
				high: high,
				low:  random.Intn(high) + 1,
			}
		}

		actual := intervalWalkMaxScores(values, edges)
		expected := bruteForceIntervalWalkScores(values, edges)
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf(
				"iteration %d: values=%v edges=%v expected=%v got=%v",
				iteration, values, edges, expected, actual,
			)
		}
	}
}

func bruteForceIntervalWalkScores(values []int, edges []intervalWalkEdge) []int {
	answers := make([]int, len(values))
	for index := range answers {
		answers[index] = -1
	}
	maximumCapacity := 0
	for _, edge := range edges {
		maximumCapacity = max(maximumCapacity, edge.high)
	}

	type state struct {
		vertex int
		height int
	}
	for start := range values {
		for initialHeight := 0; initialHeight <= maximumCapacity; initialHeight++ {
			queue := make([]state, 0)
			seen := make(map[state]bool)
			for _, edge := range edges {
				next := 0
				switch start + 1 {
				case edge.u:
					next = edge.v
				case edge.v:
					next = edge.u
				default:
					continue
				}
				if edge.high < initialHeight {
					continue
				}
				nextState := state{vertex: next - 1, height: max(initialHeight, edge.low)}
				if !seen[nextState] {
					seen[nextState] = true
					queue = append(queue, nextState)
				}
			}
			for head := 0; head < len(queue); head++ {
				current := queue[head]
				answers[start] = max(answers[start], values[current.vertex]+initialHeight)
				for _, edge := range edges {
					next := 0
					switch current.vertex + 1 {
					case edge.u:
						next = edge.v
					case edge.v:
						next = edge.u
					default:
						continue
					}
					if edge.high < current.height {
						continue
					}
					nextState := state{vertex: next - 1, height: max(current.height, edge.low)}
					if !seen[nextState] {
						seen[nextState] = true
						queue = append(queue, nextState)
					}
				}
			}
		}
	}
	return answers
}
