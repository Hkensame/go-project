# 重要
想要使用前请把所有与挂载(主要在.sh文件内),地址(在yaml配置文件中)有关的地方换成你需要的地址


# mysql
搭的mysql Docker环境,未来要实现主从复制的mysql
# project
基于go的电商微服务
# registry
私有docker库,暂时只是用来学私有库的使用
# rockmq
rocketmq的docker环境搭建,还未使用
# transfer
新:project:0.2.x已移除,后续将引入canal或者自己修改transfer
go-mysql-transfer,用于实现缓存一致性,并未找到方法解决如何主动同步缓存和缓存key的限时
# redis
搭建redis集群
# elasticsearch
用于商品,用户的搜索
