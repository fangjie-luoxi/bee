# bee
自动代码生成工具，来源与beego的bee，修改部分代码，更适合快速开发 api 接口

## 代码生成：

本代码需要：go install

## 新建api项目：

bee2 api xxx

## 更新数据库

// mac:
mysql -h localhost -P 3306 -u root --password=1234 -e "source  ~/workspace/design/xxx.sql"

// win:
mysql -h localhost -P 3306 -u root --password=root -e "source  E:\LingeWorkSpace\xxx.sql"

## 从数据库生成controllers、models、models/dto、routers：

bee2 generate appcode -driver=mysql -conn="root:root@tcp(localhost:3306)/xxx" -level=1

数据库设计：
假设当前表名为A
【一对一】A表字段中，若有X_one，且有X_id，并且数据库中存在X表，则生成A与X的一对一关系，A.X可带出X对象，X.A可带出A对象；		设计模型时：使用1对1的连线，需手工增加X_one字段
【一对多】A表字段中，若有X_id，且数据库中存在X表，则生成A与X的一对多关系，A.Xs可带出X对象集合，X.A可带出A对象；				设计模型时：使用1对n的连接线，从表自动产生关联id
【多对多】表名中，有A_has_X的，且数据库中存在A表和X表，则生成A与X的多对多关系，A.Xs可带出X对象集合，X.As可带出A对象集合；	设计模型时：使用n对m的连接线，自动产生中间表，需手工增加id字段

注意：
所有表必须有Id字段，且为主键，自增
创建时间的时间戳可以使用CURRENT_TIMESTAMP赋值
更新时间的时间戳可以使用CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP赋值
尽量避免使用 "_id"、"_one"、"_has_"、"id" 等标识命名普通字段或自定义表名
导出模型时，去掉FK(外键)的勾选


todos:
多对多关系代码生成:
Streets  []*Street `orm:"rel(m2m)"`

UpdateUserById
if m.Streets != nil {
			m2m := o.QueryM2M(&v, "Streets")
			if nums, err := m2m.Clear(); err == nil {
				fmt.Println("Removed Tag Nums: ", nums)
			}

			if num, err := m2m.Add(m.Streets); err == nil {
				fmt.Println("Added nums: ", num)
			}
		}

一对多关系代码生成:
Model *Model `orm:"rel(fk)"`
