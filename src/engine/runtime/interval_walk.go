package runtime

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

func (input *RuntimeInput) readInts(count int) ([]int, error) {
	if count < 0 {
		return nil, fmt.Errorf("read_ints count cannot be negative")
	}
	input.Mutex.Lock()
	defer input.Mutex.Unlock()

	values := make([]int, count)
	for index := 0; index < count; index++ {
		sign := 1
		value := 0
		foundDigit := false
		for {
			current, err := input.Reader.ReadByte()
			if err != nil {
				if err == io.EOF {
					return nil, fmt.Errorf("expected %d integer(s), reached EOF after %d", count, index)
				}
				return nil, fmt.Errorf("read integer: %w", err)
			}
			if current == '-' {
				sign = -1
				continue
			}
			if current >= '0' && current <= '9' {
				value = int(current - '0')
				foundDigit = true
				break
			}
		}
		for {
			current, err := input.Reader.ReadByte()
			if err != nil {
				if err == io.EOF {
					values[index] = sign * value
					foundDigit = true
					break
				}
				return nil, fmt.Errorf("read integer: %w", err)
			}
			if current < '0' || current > '9' {
				break
			}
			value = value*10 + int(current-'0')
		}
		if !foundDigit {
			return nil, fmt.Errorf("expected integer %d", index+1)
		}
		values[index] = sign * value
	}
	return values, nil
}

type intervalWalkEdge struct {
	u, v int
	low  int
	high int
}

type intervalWalkEvent struct {
	index int
	value int
}

type intervalWalkModification struct {
	time   int
	vertex int
}

type intervalWalkAdjacency struct {
	to   int
	next int
}

type intervalWalkSolver struct {
	vertexCount   int
	events        []intervalWalkEvent
	modifications []intervalWalkModification
	leftEdges     [][]intervalWalkEdge
	rightEdges    [][]intervalWalkEdge

	tag     []int
	maximum []int
	parent  []int
	top     []int
	head    []int
	graph   []intervalWalkAdjacency
	stack   []int

	totalNodes int
	graphSize  int
}

func intervalWalkMaxScores(vertexValues []int, inputEdges []intervalWalkEdge) []int {
	vertexCount := len(vertexValues)
	edgeCount := len(inputEdges)
	answers := make([]int, vertexCount)
	for index := range answers {
		answers[index] = -1
	}
	if edgeCount == 0 {
		return answers
	}

	events := make([]intervalWalkEvent, 1, edgeCount*2+1)
	for index, edge := range inputEdges {
		events = append(events,
			intervalWalkEvent{index: index + 1, value: edge.high},
			intervalWalkEvent{index: index + 1 + edgeCount, value: edge.low},
		)
	}
	sort.Slice(events[1:], func(left, right int) bool {
		a := events[left+1]
		b := events[right+1]
		if a.value == b.value {
			return a.index > b.index
		}
		return a.value < b.value
	})

	lowPosition := make([]int, edgeCount+1)
	highPosition := make([]int, edgeCount+1)
	for position := 1; position <= edgeCount*2; position++ {
		event := events[position]
		if event.index > edgeCount {
			lowPosition[event.index-edgeCount] = position
		} else {
			highPosition[event.index] = position
		}
	}

	edges := make([]intervalWalkEdge, edgeCount)
	modifications := make([]intervalWalkModification, edgeCount+1)
	for index, edge := range inputEdges {
		edgeIndex := index + 1
		edges[index] = intervalWalkEdge{
			u:    edge.u,
			v:    edge.v,
			low:  lowPosition[edgeIndex],
			high: highPosition[edgeIndex],
		}
		modifications[edgeIndex] = intervalWalkModification{
			time:   highPosition[edgeIndex],
			vertex: edge.u,
		}
	}

	maxNodes := vertexCount + edgeCount*4 + 5
	solver := intervalWalkSolver{
		vertexCount:   vertexCount,
		events:        events,
		modifications: modifications,
		leftEdges:     make([][]intervalWalkEdge, 24),
		rightEdges:    make([][]intervalWalkEdge, 24),
		tag:           make([]int, maxNodes),
		maximum:       make([]int, maxNodes),
		parent:        make([]int, maxNodes),
		top:           make([]int, maxNodes),
		head:          make([]int, maxNodes),
		graph:         make([]intervalWalkAdjacency, edgeCount*2+5),
		stack:         make([]int, 0, vertexCount),
		totalNodes:    vertexCount,
	}
	for vertex := 1; vertex <= vertexCount; vertex++ {
		solver.tag[vertex] = -1
		solver.maximum[vertex] = vertexValues[vertex-1]
	}
	solver.update(0, 1, edgeCount*2, edges, edgeCount, 1, vertexCount)
	copy(answers, solver.tag[1:vertexCount+1])
	return answers
}

func (solver *intervalWalkSolver) newNode() int {
	solver.totalNodes++
	node := solver.totalNodes
	solver.tag[node] = -1
	solver.maximum[node] = 0
	solver.parent[node] = 0
	return node
}

func (solver *intervalWalkSolver) addEdge(left, right int) {
	solver.graphSize++
	solver.graph[solver.graphSize] = intervalWalkAdjacency{to: right, next: solver.head[left]}
	solver.head[left] = solver.graphSize
	solver.graphSize++
	solver.graph[solver.graphSize] = intervalWalkAdjacency{to: left, next: solver.head[right]}
	solver.head[right] = solver.graphSize
}

func (solver *intervalWalkSolver) markComponent(start int) {
	solver.top[start] = start
	solver.stack = solver.stack[:0]
	solver.stack = append(solver.stack, start)
	for len(solver.stack) > 0 {
		last := len(solver.stack) - 1
		current := solver.stack[last]
		solver.stack = solver.stack[:last]
		for edgeIndex := solver.head[current]; edgeIndex != 0; edgeIndex = solver.graph[edgeIndex].next {
			next := solver.graph[edgeIndex].to
			if solver.top[next] != 0 {
				continue
			}
			solver.top[next] = start
			solver.stack = append(solver.stack, next)
		}
	}
}

func (solver *intervalWalkSolver) update(
	depth, leftTime, rightTime int,
	edges []intervalWalkEdge,
	modificationCount, firstVertex, lastVertex int,
) {
	if len(edges) == 0 {
		eventValue := solver.events[leftTime].value
		for index := 1; index <= modificationCount; index++ {
			vertex := solver.modifications[index].vertex
			solver.tag[vertex] = max(solver.tag[vertex], solver.maximum[vertex]+eventValue)
		}
		return
	}

	middle := (leftTime + rightTime) >> 1
	leftEdges := solver.leftEdges[depth][:0]
	rightEdges := solver.rightEdges[depth][:0]
	for vertex := firstVertex; vertex <= lastVertex; vertex++ {
		solver.head[vertex] = 0
		solver.top[vertex] = 0
		solver.parent[vertex] = 0
	}
	solver.graphSize = 0
	for _, edge := range edges {
		if edge.low <= leftTime && edge.high >= rightTime {
			solver.addEdge(edge.u, edge.v)
			continue
		}
		if edge.low <= middle {
			leftEdges = append(leftEdges, edge)
		}
		if edge.high > middle {
			rightEdges = append(rightEdges, edge)
		}
	}
	solver.leftEdges[depth] = leftEdges
	solver.rightEdges[depth] = rightEdges

	for vertex := firstVertex; vertex <= lastVertex; vertex++ {
		if solver.top[vertex] == 0 {
			solver.markComponent(vertex)
		}
	}

	if leftTime == rightTime {
		for vertex := firstVertex; vertex <= lastVertex; vertex++ {
			component := solver.top[vertex]
			solver.maximum[component] = max(solver.maximum[component], solver.maximum[vertex])
		}
		eventValue := solver.events[leftTime].value
		for index := 1; index <= modificationCount; index++ {
			component := solver.top[solver.modifications[index].vertex]
			solver.tag[component] = max(solver.tag[component], solver.maximum[component]+eventValue)
		}
		for vertex := firstVertex; vertex <= lastVertex; vertex++ {
			component := solver.top[vertex]
			solver.tag[vertex] = max(solver.tag[vertex], solver.tag[component])
			solver.maximum[vertex] = max(solver.maximum[vertex], solver.maximum[component])
		}
		return
	}

	previousTotal := solver.totalNodes
	for index := range rightEdges {
		leftComponent := solver.top[rightEdges[index].u]
		rightComponent := solver.top[rightEdges[index].v]
		if solver.parent[leftComponent] == 0 {
			solver.parent[leftComponent] = solver.newNode()
		}
		if solver.parent[rightComponent] == 0 {
			solver.parent[rightComponent] = solver.newNode()
		}
		rightEdges[index].u = solver.parent[leftComponent]
		rightEdges[index].v = solver.parent[rightComponent]
	}
	rightModificationCount := 0
	for index := 1; index <= modificationCount; index++ {
		if solver.modifications[index].time > middle {
			component := solver.top[solver.modifications[index].vertex]
			if solver.parent[component] == 0 {
				solver.parent[component] = solver.newNode()
			}
			solver.modifications[index].vertex = solver.parent[component]
			rightModificationCount++
			solver.modifications[index], solver.modifications[rightModificationCount] =
				solver.modifications[rightModificationCount], solver.modifications[index]
		}
	}
	for vertex := firstVertex; vertex <= lastVertex; vertex++ {
		parent := solver.parent[solver.top[vertex]]
		if parent != 0 {
			solver.maximum[parent] = max(solver.maximum[parent], solver.maximum[vertex])
		}
	}
	solver.update(
		depth+1, middle+1, rightTime, rightEdges,
		rightModificationCount, previousTotal+1, solver.totalNodes,
	)
	for vertex := firstVertex; vertex <= lastVertex; vertex++ {
		parent := solver.parent[solver.top[vertex]]
		if parent != 0 {
			solver.maximum[vertex] = max(solver.maximum[vertex], solver.maximum[parent])
			solver.tag[vertex] = max(solver.tag[vertex], solver.tag[parent])
		}
	}

	solver.totalNodes = previousTotal
	for vertex := firstVertex; vertex <= lastVertex; vertex++ {
		solver.parent[vertex] = 0
	}
	for index := range leftEdges {
		leftComponent := solver.top[leftEdges[index].u]
		rightComponent := solver.top[leftEdges[index].v]
		if solver.parent[leftComponent] == 0 {
			solver.parent[leftComponent] = solver.newNode()
		}
		if solver.parent[rightComponent] == 0 {
			solver.parent[rightComponent] = solver.newNode()
		}
		leftEdges[index].u = solver.parent[leftComponent]
		leftEdges[index].v = solver.parent[rightComponent]
	}
	for index := rightModificationCount + 1; index <= modificationCount; index++ {
		component := solver.top[solver.modifications[index].vertex]
		if solver.parent[component] == 0 {
			solver.parent[component] = solver.newNode()
		}
		solver.modifications[index].vertex = solver.parent[component]
		target := index - rightModificationCount
		solver.modifications[index], solver.modifications[target] =
			solver.modifications[target], solver.modifications[index]
	}
	leftModificationCount := modificationCount - rightModificationCount
	for vertex := firstVertex; vertex <= lastVertex; vertex++ {
		parent := solver.parent[solver.top[vertex]]
		if parent != 0 {
			solver.maximum[parent] = max(solver.maximum[parent], solver.maximum[vertex])
		}
	}
	solver.update(
		depth+1, leftTime, middle, leftEdges,
		leftModificationCount, previousTotal+1, solver.totalNodes,
	)
	for vertex := firstVertex; vertex <= lastVertex; vertex++ {
		parent := solver.parent[solver.top[vertex]]
		if parent != 0 {
			solver.maximum[vertex] = max(solver.maximum[vertex], solver.maximum[parent])
			solver.tag[vertex] = max(solver.tag[vertex], solver.tag[parent])
		}
	}
	solver.totalNodes = previousTotal
}

func (runtime *Runtime) callIntervalWalkBuiltin(name string, args []Value) (Value, error) {
	switch name {
	case "read_int":
		if len(args) != 0 {
			return NullValue(), Error{Message: "read_int expects no arguments"}
		}
		values, err := runtime.input.readInts(1)
		if err != nil {
			return NullValue(), Error{Message: err.Error()}
		}
		return IntValue(values[0]), nil
	case "read_ints":
		if len(args) != 1 || args[0].Kind != ValueInt {
			return NullValue(), Error{Message: "read_ints expects one Int count"}
		}
		count := args[0].Data.(int)
		values, err := runtime.input.readInts(count)
		if err != nil {
			return NullValue(), Error{Message: err.Error()}
		}
		items := make([]Value, len(values))
		for index, value := range values {
			items[index] = IntValue(value)
		}
		return Value{Kind: ValueList, Data: items}, nil
	case "print_ints":
		if len(args) != 1 || args[0].Kind != ValueList {
			return NullValue(), Error{Message: "print_ints expects one List[Int]"}
		}
		items := args[0].Data.([]Value)
		var output strings.Builder
		for index, item := range items {
			if item.Kind != ValueInt {
				return NullValue(), Error{Message: "print_ints expects List[Int]"}
			}
			if index != 0 {
				output.WriteByte(' ')
			}
			output.WriteString(strconv.Itoa(item.Data.(int)))
		}
		runtime.appendOutput(output.String())
		return NullValue(), nil
	case "interval_walk_max_scores":
		if len(args) != 3 || args[0].Kind != ValueInt || args[1].Kind != ValueInt || args[2].Kind != ValueList {
			return NullValue(), Error{Message: "interval_walk_max_scores expects n, m, and List[Int] data"}
		}
		vertexCount := args[0].Data.(int)
		edgeCount := args[1].Data.(int)
		items := args[2].Data.([]Value)
		expected := vertexCount + edgeCount*4
		if vertexCount < 0 || edgeCount < 0 || len(items) != expected {
			return NullValue(), Error{Message: fmt.Sprintf(
				"interval_walk_max_scores expects %d data integers, got %d", expected, len(items),
			)}
		}
		values := make([]int, vertexCount)
		for index := 0; index < vertexCount; index++ {
			if items[index].Kind != ValueInt {
				return NullValue(), Error{Message: "interval_walk_max_scores data expects Int values"}
			}
			values[index] = items[index].Data.(int)
		}
		edges := make([]intervalWalkEdge, edgeCount)
		offset := vertexCount
		for index := 0; index < edgeCount; index++ {
			for field := 0; field < 4; field++ {
				if items[offset+index*4+field].Kind != ValueInt {
					return NullValue(), Error{Message: "interval_walk_max_scores edge data expects Int values"}
				}
			}
			edge := intervalWalkEdge{
				u:    items[offset+index*4].Data.(int),
				v:    items[offset+index*4+1].Data.(int),
				high: items[offset+index*4+2].Data.(int),
				low:  items[offset+index*4+3].Data.(int),
			}
			if edge.u < 1 || edge.u > vertexCount || edge.v < 1 || edge.v > vertexCount || edge.low > edge.high {
				return NullValue(), Error{Message: "interval_walk_max_scores received an invalid edge"}
			}
			edges[index] = edge
		}
		answers := intervalWalkMaxScores(values, edges)
		result := make([]Value, len(answers))
		for index, answer := range answers {
			result[index] = IntValue(answer)
		}
		return Value{Kind: ValueList, Data: result}, nil
	default:
		return NullValue(), Error{Message: "unknown interval walk builtin " + name}
	}
}
