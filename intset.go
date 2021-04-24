package clist

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

type IntSet interface {
	// 检查一个元素是否存在，如果存在则返回 true，否则返回 false
	Contains(value int) bool

	// 插入一个元素，如果此操作成功插入一个元素，则返回 true，否则返回 false
	Insert(value int) bool

	// 删除一个元素，如果此操作成功删除一个元素，则返回 true，否则返回 false
	Delete(value int) bool

	// 遍历此有序链表的所有元素，如果 f 返回 false，则停止遍历
	Range(f func(value int) bool)

	// 返回有序链表的元素个数
	Len() int
}

const (
	nodeNotDeleted = uint32(0) // 逻辑未删除
	nodeDeleted    = uint32(1) // 逻辑删除
)

type Node struct {
	value  int
	marked uint32
	next   *Node
	mu     sync.Mutex
}

func newNode(value int, next *Node) *Node {
	return &Node{
		value: value,
		next:  next,
	}
}

// 原子操作 - 设置逻辑删除flag
func (n *Node) setDeleted() {
	atomic.StoreUint32(&n.marked, nodeDeleted)
}

// 原子操作 - 检查是否逻辑删除
func (n *Node) isDeleted() bool {
	return atomic.LoadUint32(&n.marked) == nodeDeleted
}

// 原子操作 - 保存后续节点
func (n *Node) storeNext(node *Node) {
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&n.next)), unsafe.Pointer(node))
}

// 原子操作 - 读取后续节点
func (n *Node) loadNext() *Node {
	return (*Node)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&n.next))))
}

type IntList struct {
	head *Node
	len  int32
}

func (i *IntList) Contains(value int) bool {
	c := i.head.loadNext()
	for c != nil {
		// 假定有序链表是依次增大的，如果判断的目标值小于当前节点的值，就表示没有存在的可能了，直接返回not contains
		if value < c.value {
			return false
		}

		if value != c.value {
			c = c.loadNext()
			continue
		}

		// 检查是否有逻辑删flag
		return !c.isDeleted()
	}

	return false
}

func (i *IntList) Insert(value int) bool {
	// Step 1: 找到前序/后续节点
	var found bool
	var a, b *Node

	for !found {
		a, b = i.findPositions(value)

		// 检查待写入值是否已存在
		if b != nil && b.value == value {
			return false
		}

		// Step 2: 锁定节点 A，检查 A.next == B，如果为假，则解锁 A 然后返回 Step 1
		a.mu.Lock()

		if a.next != b {
			a.mu.Unlock()
			continue
		}

		// 确保左右节点均有效
		if a.isDeleted() || (b != nil && b.isDeleted()) {
			a.mu.Unlock()
			continue
		}

		found = true
	}

	defer a.mu.Unlock()

	// Step 3: 创建新节点X，调整指向关系
	x := newNode(value, b)
	a.storeNext(x)

	// 更新len
	atomic.AddInt32(&i.len, 1)

	return true
}

func (i *IntList) Delete(value int) bool {
	// Step 1: 找到前序/后续节点
	var found bool
	var a, b *Node

	for !found {
		a, b = i.findPositions(value)

		if b == nil || b.value != value {
			return false
		}

		// Step 2: 锁定节点B，检查逻辑删除flag
		b.mu.Lock()
		if b.isDeleted() {
			b.mu.Unlock()

			continue
		}

		// Step 3: 锁定节点A，检查逻辑删除flag以及邻接关系
		a.mu.Lock()
		if a.loadNext() != b || a.isDeleted() {
			a.mu.Unlock()
			b.mu.Unlock()

			continue
		}

		found = true
	}

	defer b.mu.Unlock()
	defer a.mu.Unlock()

	// Step 4: 删除b；改变邻接关系
	b.setDeleted()
	a.storeNext(b.loadNext())

	// 更新len
	atomic.AddInt32(&i.len, -1)

	return true
}

func (i *IntList) Range(f func(value int) bool) {
	x := i.head.loadNext()
	for x != nil {
		if !f(x.value) {
			break
		}
		x = x.loadNext()
	}

}

func (i *IntList) Len() int {
	return int(atomic.LoadInt32(&i.len))
}

func NewInt() *IntList {
	return &IntList{head: newNode(-1, nil)}
}

// ~ 私有方法

// 寻找目标值的邻接节点
// 这里注意一定要是ptr receiver形式，否则会有data race问题，参考readme中的解释
func (i *IntList) findPositions(value int) (a, b *Node) {
	a = i.head
	b = a.loadNext()

	for b != nil && b.value < value {
		a = b
		b = a.loadNext()
	}

	return
}
