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

@trim("and") {
    @for _,v := range ids {
        and id = @v
    }
}

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

and a = @ Test() {
    select ok
}

@use test.sql4.a {

}
```

## sql6
测试自定义函数块
```sql
select * from table
where 1 = 1

@trim("and") {
    @for _,v := range ids {
        and id = @v
    }
}
```


## sql7
12312321
```sql
select * from @GetTable()
where 
    1 = 1
    and id = @id
    and name = @GetName()
    and name = @=GetName()

@if id > 0 {
    and a = 1
} else if {
    and b = 2
} else {
    and c = 3
}

--# slot abc 
--# end

@define abc {
    and id = @id
    and id2 = @=Id
}

--# trim ,
--# end

```


## sql8
```sql
@use test.sql7.abc {
}
;

select * from table 
where
@Trim("and") {
    @for _,v := range ids {
        and id = @v
    }
}

@UseV2("") {

}

@UseV3("") {

}
```