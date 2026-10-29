package main

// 本目录（cmd/openapi）与 cmd/contractcheck 共同构成 CMDB 前后端契约治理闭环。
//
// 背景：e-cmdb-web 的 src/api/types/cmdb.ts 为人工手写，与 e-cmdb-service 的
// struct 之间没有任何机械校验。历史上已因此产生 4 次字段名不一致缺陷
// （uid/model_uid x3、source_uid/source_model_uid x1），表现为「页面字段空白」
// 或「提交 400」，且每次都需人工逐页排查。
//
// 两个命令：
//
//	cmd/openapi       后端源码 -> OpenAPI 3.0 规范（新增/变更接口后重新生成）
//	cmd/contractcheck 规范 <-> 前端 TS 类型 一致性校验（CI 门禁）
//
// 典型用法：
//
//	go run ./cmd/openapi -root . -o openapi.json
//	go run ./cmd/contractcheck -spec openapi.json -ts ../e-cmdb-web/src/api/types/cmdb.ts
//
// 设计取舍：
//   - 请求体类型从路由注册的泛型参数（ginx.WrapBody[T]）静态提取，覆盖率 100%，
//     与实现天然同源，不会腐化。
//   - 响应类型从 handler 的 return Result(...) 实参推断：具名结构体字面量可
//     精确到 $ref，内联 map 可提取 key，Result(nil) 标记为无数据。少数间接
//     构造（如经服务层转换的 DTO）无法静态确定，此时留空而非臆造——宁可缺省
//     也不生成错误契约。
//   - 不引入 swaggo 等注解框架：本项目 handler 为泛型包装风格，注解会与实现
//     分离且需为 60+ 路由手工维护，反而增加腐化面。
