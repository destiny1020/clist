# clist
一个支持并发操作的有序列表实现

# 运行

## 运行测试用例
```
go test
```

一个可能的运行结果为：

```
➜ clist git:(main) ✗ go test
PASS
ok      github.com/destiny1020/clist    0.903s
```

## 使用race check
```
go test -race
```

一个可能的运行结果为：

```
➜ clist git:(main) ✗ go test -race
PASS
ok      github.com/destiny1020/clist    19.580s
```

# 使用说明

可以参考intset_test.go中的各种用法。已经包含了完善的串行、并发使用方式例子。

# 踩坑记录

## 未使用atomic
在并发一写多读的区域，需要使用 atomic.Load 来读取相应的值，使⽤ atomic.Store 来存储相应的值。

这个就不多说了，再仔细阅读课件后，注意到了这个点。:)

## 抽取公用部分时，receiver未使用指针类型
```go
func (i *IntList) findPositions(value int) (a, b *Node) {
    a = i.head
    b = a.loadNext()

    for b != nil && b.value < value {
        a = b
        b = a.loadNext()
    }

    return
}
```
以上私有方法findPositions的receiver一开始未设置为指针类型。但是race检查时发现了data race：
```
➜ clist git:(main) ✗ go test -race
==================
WARNING: DATA RACE
Read at 0x00c000092478 by goroutine 10:
  github.com/destiny1020/clist.(*IntList).Insert()
      /Users/destiny1020/repos/opensource/clist/intset.go:96 +0x84
  github.com/destiny1020/clist.TestIntSet.func4()
      /Users/destiny1020/repos/opensource/clist/intset_test.go:115 +0x70

Previous write at 0x00c000092478 by goroutine 9:
  sync/atomic.AddInt32()
      /usr/local/Cellar/go@1.13/1.13.8/libexec/src/runtime/race_amd64.s:269 +0xb
  github.com/destiny1020/clist.(*IntList).Insert()
      /Users/destiny1020/repos/opensource/clist/intset.go:127 +0x30f
  github.com/destiny1020/clist.TestIntSet.func4()
      /Users/destiny1020/repos/opensource/clist/intset_test.go:115 +0x70

Goroutine 10 (running) created at:
  github.com/destiny1020/clist.TestIntSet()
      /Users/destiny1020/repos/opensource/clist/intset_test.go:114 +0x6c8
  testing.tRunner()
      /usr/local/Cellar/go@1.13/1.13.8/libexec/src/testing/testing.go:909 +0x199

Goroutine 9 (finished) created at:
  github.com/destiny1020/clist.TestIntSet()
      /Users/destiny1020/repos/opensource/clist/intset_test.go:114 +0x6c8
  testing.tRunner()
      /usr/local/Cellar/go@1.13/1.13.8/libexec/src/testing/testing.go:909 +0x199
==================
```

这里提示： a, b = i.findPositions(value) 和 atomic.AddInt32(&i.len, 1) 发生了data race。
一个是读取操作，一个是写入操作。它们为何会有冲突呢？

会不会和使用值receiver时，存在的值拷贝有关系？带着这个疑虑，发现还真是，参考：

http://tleyden.github.io/blog/2016/05/19/go-race-detector-gotcha-with-value-receivers/
https://stackoverflow.com/questions/42034178/data-race-when-reading-field-from-struct-value-passed-by-value

总结一下原因：

1. 因为使用值receiver时，会有一个拷贝结构体中所有fields的动作，也就是需要read所有fields，包含IntList中的head以及len两个字段，
   虽然明面上，findPositions内部的逻辑和len压根就没有半点关系。
2. 而值拷贝过程中对len字段的读取和另外一个goroutine对len的并发写入产生了data race。
3. 所以这也解释了，上面检测输出中是findPositions方法的调用行，而不是方法内部的某一行，因为这里隐藏着对len字段的读取。

所以替换成指针receiver后，上述问题迎刃而解。因为调用过程不涉及到结构体字段的拷贝，也就没有了读取操作。