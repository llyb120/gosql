# test

## sql1
基础功能
```sql
select * from table 
where 
    id = @id
    -- 数组的情况
    and id in (@ids)
    -- 直接输出的情况
    and id = @=id


```

## sql2 
流程控制
```sql
select * from table
where id = 1
    and name = @name?
    and age = @age?
    and id = @id?

@for i := 0; i < 2; i++ {
    this is for item: id = @id
}

for end

@for i,v := range ids {
    this is for item: id = @v
}
```

## sql3
use function
```sql 
select * from 
-- 基础use
@use test.sql4 {
    @cover a {
        and id <> @id
    }
}
-- usedefine的情况
@use test.sql4 {
    @cover b {
        and id = @id
    }
}

@ GetName() {
    select ok
}
```

## sql4
define 
```sql
select * from table
where 1 = 1
@define a {
    this is block a
    and id = @id
    @define b {

    }
}
```


## sql5
```sql
name is @= GetName() @
id is @= GetId() @

@use test.sql4.a {

}
```