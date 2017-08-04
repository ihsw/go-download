package Cache

import (
	"fmt"
	"github.com/ihsw/go-download/Config"
	redis "gopkg.in/redis.v2"
	"strconv"
)

/*
	funcs
*/
func NewWrapper(c Config.Connection) (w Wrapper, err error) {
	var r *redis.Client
	if r, err = c.Connect(); err != nil {
		return
	}

	return Wrapper{Redis: r}, nil
}

/*
	interfaces
*/
type Manager interface {
	Namespace() string
}

/*
	PersistValue
*/
type PersistValue struct {
	BucketKey string
	SubKey    string
	Value     string
}

/*
	Wrapper
*/
type Wrapper struct {
	Redis *redis.Client
}

func (self Wrapper) getCacheKey(bucketKey string, subKey string) string {
	return fmt.Sprintf("%s-%s", bucketKey, subKey)
}

func (self Wrapper) FetchIds(key string, start int64, end int64) (ids []int64, err error) {
	req := self.Redis.LRange(key, start, end)
	if err = req.Err(); err != nil {
		return
	}

	// optionally halting
	length := len(req.Val())
	if length == 0 {
		return ids, nil
	}

	// converting them
	ids = make([]int64, length)
	var i int
	for k, v := range req.Val() {
		i, err = strconv.Atoi(v)
		if err != nil {
			return
		}
		ids[k] = int64(i)
	}

	return ids, nil
}

func (self Wrapper) FetchFromId(manager Manager, id int64) (v string, err error) {
	// misc
	var values []string

	// forwarding to the FetchFromIds method
	ids := make([]int64, 1)
	ids[0] = id
	values, err = self.FetchFromIds(manager, ids)
	if err != nil {
		return
	}

	return values[0], nil
}

func (self Wrapper) FetchFromIds(manager Manager, ids []int64) (values []string, err error) {
	// misc
	idsLength := len(ids)

	// optionally halting on empty ids list
	if idsLength == 0 {
		return values, err
	}

	// gathering input from the ids
	r := self.Redis
	values = make([]string, idsLength)
	for i, id := range ids {
		bucketKey, subKey := GetBucketKey(id, manager.Namespace())
		cmd := r.HGet(bucketKey, subKey)
		if err = cmd.Err(); err != nil && err != redis.Nil {
			return
		}
		values[i] = cmd.Val()
	}

	return values, nil
}

func (self Wrapper) FetchFromKey(manager Manager, k string) (v string, err error) {
	// checking for an id
	v, err = self.Get(k)
	if err != nil {
		return
	}
	if len(v) == 0 {
		return
	}

	// checking for data
	var id int64
	id, err = strconv.ParseInt(v, 10, 64)
	if err != nil {
		return
	}
	return self.FetchFromId(manager, id)
}

func (self Wrapper) Persist(bucketKey string, subKey string, value string) (err error) {
	cmd := self.Redis.HSet(bucketKey, subKey, value)
	if err = cmd.Err(); err != nil {
		return
	}

	return nil
}

func (self Wrapper) IncrAll(key string, count int) (ids []int64, err error) {
	// misc
	var cmds []redis.Cmder
	pipe := self.Redis.Pipeline()

	// running the pipeline
	for i := 0; i < count; i++ {
		pipe.Incr(key)
	}
	cmds, err = pipe.Exec()
	if err != nil {
		return
	}

	// gathering for ids and checking for errors
	ids = make([]int64, len(cmds))
	for i, cmd := range cmds {
		if err = cmd.Err(); err != nil {
			return
		}

		ids[i] = cmd.(*redis.IntCmd).Val()
	}

	return ids, nil
}

func (self Wrapper) PersistAll(values []PersistValue) (err error) {
	// misc
	var cmds []redis.Cmder
	pipe := self.Redis.Pipeline()

	// running the pipeline
	for _, v := range values {
		pipe.HSet(v.BucketKey, v.SubKey, v.Value)
	}
	cmds, err = pipe.Exec()
	if err != nil {
		return
	}

	// checking for errors
	for _, cmd := range cmds {
		if err = cmd.Err(); err != nil {
			return
		}
	}

	return nil
}

func (self Wrapper) RPushAll(key string, values []string) (err error) {
	// misc
	var cmds []redis.Cmder
	pipe := self.Redis.Pipeline()

	// running the pipeline
	for _, v := range values {
		pipe.RPush(key, v)
	}
	cmds, err = pipe.Exec()
	if err != nil {
		return
	}

	// checking for errors
	for _, cmd := range cmds {
		if err = cmd.Err(); err != nil {
			return
		}
	}

	return nil
}

func (self Wrapper) SAddAll(key string, values []string) (err error) {
	// misc
	var cmds []redis.Cmder
	pipe := self.Redis.Pipeline()

	// running the pipeline
	for _, v := range values {
		pipe.SAdd(key, v)
	}
	cmds, err = pipe.Exec()
	if err != nil {
		return
	}

	// checking for errors
	for _, cmd := range cmds {
		if err = cmd.Err(); err != nil {
			return
		}
	}

	return nil
}

func (self Wrapper) SMembers(key string) (members []string, err error) {
	cmd := self.Redis.SMembers(key)
	if err = cmd.Err(); err != nil {
		return
	}
	return cmd.Val(), nil
}

func (self Wrapper) SIsMember(key string, value string) (isMember bool, err error) {
	values := []string{value}
	var isMembers []bool
	if isMembers, err = self.SIsMemberAll(key, values); err != nil {
		return
	}

	return isMembers[0], nil
}

func (self Wrapper) SIsMemberAll(key string, values []string) (isMembers []bool, err error) {
	var cmds []redis.Cmder
	pipe := self.Redis.Pipeline()

	// running the pipeline
	for _, v := range values {
		pipe.SIsMember(key, v)
	}
	if cmds, err = pipe.Exec(); err != nil {
		return
	}

	// checking for errors
	isMembers = make([]bool, len(cmds))
	for i, cmd := range cmds {
		if err = cmd.Err(); err != nil {
			return
		}

		isMembers[i] = cmd.(*redis.BoolCmd).Val()
	}

	return isMembers, nil
}

func (self Wrapper) SetAll(values map[string]string) (err error) {
	var cmds []redis.Cmder
	pipe := self.Redis.Pipeline()

	// running the pipeline
	for k, v := range values {
		pipe.Set(k, v)
	}
	if cmds, err = pipe.Exec(); err != nil {
		return
	}

	// checking for errors
	for _, cmd := range cmds {
		if err = cmd.Err(); err != nil {
			return
		}
	}

	return nil
}

func (self Wrapper) Get(k string) (v string, err error) {
	cmd := self.Redis.Get(k)
	if err = cmd.Err(); err != nil && err != redis.Nil {
		return
	}

	return cmd.Val(), nil
}
