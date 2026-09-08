# KeystoneGo 业务实现代码速查笔记

> 适合用途：学习项目、复习 Gin/GORM、以后开发相似功能时复制思路。
>
> 本文以当前 KeystoneGo 源码为基础。标有“当前实现”的代码是在解释项目现在怎么运行；标有“推荐写法”的代码用于提醒你不要把项目里尚未闭环的地方直接复制到新功能。

## 目录

1. 项目业务地图
2. 一次请求的完整调用链
3. 参数到底从哪里来
4. `nil` 和 `error` 判断速记
5. 统一响应和错误处理
6. 注册业务
7. 用户名密码登录
8. JWT 认证和 Casbin 鉴权
9. Token 刷新与退出登录
10. 短信验证码与短信登录
11. GitHub OAuth 登录
12. 登录渠道绑定、解绑
13. 找回和重置密码
14. 当前用户资料与头像
15. 用户管理业务
16. 角色管理业务
17. 菜单管理业务
18. 权限定义与 Casbin 权限的区别
19. 系统监控、健康检查和中间件
20. 配置加载和 YAML 关联
21. 缓存、队列、熔断器等预留能力
22. 前端 Token 自动刷新
23. 当前项目需要注意的问题及推荐写法
24. 新增一个业务接口的完整模板
25. 开发新需求时的检查清单

---

## 1. 项目业务地图

KeystoneGo 由一个 Go 后端和两个 Vue 前端组成：

```text
frontend       管理后台：用户、角色、菜单、权限、监控
user-frontend  普通用户端：注册、登录、个人资料、账号绑定
backend        所有真实业务、数据库、认证和权限判断
```

后端真正注册并可以访问的业务分为以下几类：

```text
公开业务
├── 用户名密码注册、登录
├── 短信发送、短信登录
├── GitHub OAuth 登录
├── Refresh Token 刷新
└── 找回、重置密码

登录用户业务
├── 获取当前用户信息
├── 修改个人资料
├── 上传头像
├── 查询、绑定、解绑登录渠道
├── 查询当前用户菜单
└── 退出登录

管理后台业务
├── 用户管理
├── 角色管理
├── 菜单管理
├── 权限定义管理
└── 系统监控
```

后端路由统一注册在：

```go
// backend/cmd/server/wire.go

// 公开接口：不需要 Access Token
r.POST("/api/v1/auth/login", authHandler.Login)
r.POST("/api/v1/auth/register", authHandler.Register)
r.POST("/api/v1/auth/refresh", authHandler.Refresh)
r.POST("/api/v1/auth/sms/send", authHandler.SendSMS)
r.POST("/api/v1/auth/sms/login", authHandler.SMSLogin)
r.GET("/api/v1/auth/oauth/github", authHandler.GitHubOAuth)
r.GET("/api/v1/auth/oauth/github/callback", authHandler.GitHubCallback)
r.POST("/api/v1/auth/password/forgot", authHandler.ForgotPassword)
r.POST("/api/v1/auth/password/reset", authHandler.ResetPassword)

// 下面这个分组里的接口全部需要 JWT + Casbin 权限
v1 := r.Group("/api/v1")
v1.Use(middleware.JWTAuth())
{
    v1.GET("/user/info", userHandler.GetCurrentUser)
    v1.GET("/user/profile", userHandler.GetProfile)
    v1.PUT("/user/profile", userHandler.UpdateProfile)
    v1.POST("/user/avatar", userHandler.UploadAvatar)
    v1.POST("/auth/logout", authHandler.Logout)
    v1.GET("/auth/channels", authHandler.GetChannels)
    v1.POST("/auth/bind/phone", authHandler.BindPhone)
    v1.POST("/auth/unbind", authHandler.Unbind)
    v1.GET("/user/menus", menuHandler.GetUserMenus)
}
```

旧版兼容路径 `/api/login` 和 `/api/register` 仍然存在，它们与新版接口调用相同的 Handler。

---

## 2. 一次请求的完整调用链

以后看任何接口，都按下面的顺序寻找：

```text
前端发请求
  ↓
Router：URL 应该交给哪个 Handler
  ↓
Middleware：日志、限流、JWT、Casbin
  ↓
Handler：接收参数、调用 Service、返回 JSON
  ↓
Service：判断业务规则、组织执行步骤
  ↓
Repository：使用 GORM 操作数据库
  ↓
Model：定义数据库字段和关联关系
```

以“查询用户列表”为例：

```go
// 1. Router：把 GET /api/v1/users 交给 userHandler.List
users.GET("", userHandler.List)

// 2. Handler：从 URL Query 获取参数，然后调用 Service
func (h *UserHandler) List(c *gin.Context) {
    var req service.UserListReq
    c.ShouldBindQuery(&req)

    users, total, err := h.userSvc.List(req)
    if err != nil {
        response.Fail(c, response.ErrSystemError, err.Error())
        return
    }

    response.Success(c, gin.H{
        "list":  users,
        "total": total,
    })
}

// 3. Service：补充分页默认值
func (s *UserService) List(req UserListReq) ([]model.User, int64, error) {
    if req.Page <= 0 {
        req.Page = 1
    }
    if req.PageSize <= 0 {
        req.PageSize = 10
    }
    return s.userRepo.ListWithKeyword(req.Keyword, req.Page, req.PageSize)
}

// 4. Repository：拼接 GORM 查询并访问 MySQL
func (r *UserRepository) ListWithKeyword(
    keyword string,
    page int,
    pageSize int,
) ([]model.User, int64, error) {
    var users []model.User
    var total int64

    q := r.db.Model(&model.User{})
    if keyword != "" {
        like := "%" + keyword + "%"
        q = q.Where(
            "username LIKE ? OR nickname LIKE ? OR email LIKE ?",
            like, like, like,
        )
    }

    // Count 查符合条件的总记录数，供前端分页使用
    if err := q.Count(&total).Error; err != nil {
        return nil, 0, err
    }

    offset := (page - 1) * pageSize
    err := q.Offset(offset).
        Limit(pageSize).
        Order("id DESC").
        Find(&users).Error

    return users, total, err
}
```

关键记忆：Handler 不直接写 SQL，Repository 不负责返回 HTTP JSON，业务判断优先放在 Service。

---

## 3. 参数到底从哪里来

Gin 不会先通过某个自定义中间件转换前端 JSON。真正的 JSON 解析发生在 `c.ShouldBindJSON(&req)`。

### 3.1 JSON 请求体

前端发送：

```json
{
  "username": "zhangsan",
  "password": "123456"
}
```

后端结构体：

```go
type LoginReq struct {
    // json:"username" 表示读取 JSON 中的 username
    // binding:"required" 表示不能为空
    Username string `json:"username" binding:"required"`

    Password string `json:"password" binding:"required"`
}
```

绑定代码：

```go
func (h *AuthHandler) Login(c *gin.Context) {
    var req LoginReq

    // ShouldBindJSON 做两件事：
    // 1. 读取 HTTP Request Body
    // 2. 按 json tag 把字段写入 req
    if !response.ShouldBindJSON(c, &req) {
        return
    }

    // 运行到这里时，req.Username 已经有前端传入的值
    zap.L().Info("进入登录接口",
        zap.String("username", req.Username),
        // 不要打印密码、Token、验证码等敏感信息
    )

    pair, user, err := h.authSvc.Login(req.Username, req.Password)
    // ...
}
```

### 3.2 URL 路径参数

请求：

```text
GET /api/v1/users/15
```

路由和读取方法：

```go
users.GET("/:id", userHandler.Get)

func (h *UserHandler) Get(c *gin.Context) {
    idText := c.Param("id") // 得到字符串 "15"

    id, err := strconv.ParseUint(idText, 10, 64)
    if err != nil {
        response.Fail(c, response.ErrParamInvalid, "用户 ID 格式错误")
        return
    }

    user, err := h.userSvc.GetByID(id)
    // ...
}
```

### 3.3 URL 查询参数

请求：

```text
GET /api/v1/users?keyword=张&page=1&page_size=10
```

读取方法：

```go
type UserListReq struct {
    Keyword  string `form:"keyword"`
    Page     int    `form:"page"`
    PageSize int    `form:"page_size"`
}

var req UserListReq
if err := c.ShouldBindQuery(&req); err != nil {
    response.Fail(c, response.ErrParamInvalid, "查询参数错误")
    return
}
```

### 3.4 请求头

```go
// 获取 Authorization: Bearer xxx
authHeader := c.GetHeader("Authorization")

// 获取前端或上游传来的链路 ID
traceID := c.GetHeader("X-Trace-Id")
```

### 3.5 中间件写入的上下文参数

JWT 中间件解析 Token 后写入：

```go
c.Set("current_user_id", claims.UserID)
c.Set("current_username", claims.Username)
c.Set("current_user_roles", claims.Roles)
c.Set("current_token_jti", claims.JTIShort)
```

Handler 读取：

```go
value, exists := c.Get("current_user_id")
if !exists {
    response.Fail(c, response.ErrUnauthorized, "未登录")
    return
}

userID, ok := value.(uint64)
if !ok {
    response.Fail(c, response.ErrSystemError, "用户上下文类型错误")
    return
}
```

### 3.6 文件参数

前端使用 `multipart/form-data` 上传：

```go
file, err := c.FormFile("avatar")
if err != nil {
    response.Fail(c, response.ErrParamInvalid, "请选择头像文件")
    return
}
```

---

## 4. `nil` 和 `error` 判断速记

### 4.1 `err == nil`

表示没有发生错误。

```go
user, err := repo.FindByUsername(username)

if err == nil {
    // 查询成功，user 是查到的用户
}
```

注册时判断用户名是否存在：

```go
if _, err := s.userRepo.FindByUsername(username); err == nil {
    // 没有错误 = 数据库成功查到这个用户名
    // 所以不能再注册
    return nil, errors.New("用户名已存在")
}
```

### 4.2 `err != nil`

表示发生了错误，但还要继续判断是哪种错误。

```go
user, err := s.userRepo.FindByUsername(username)
if err != nil {
    if errors.Is(err, gorm.ErrRecordNotFound) {
        // 这是一种可以预期的业务情况：没有查到记录
        return nil, errors.New("用户不存在")
    }

    // 其他错误可能是数据库断开、SQL 出错等系统问题
    return nil, err
}
```

### 4.3 返回多个值时，`err` 来自哪里

```go
func RefreshAccessToken(token string) (*TokenPair, error) {
    if tokenTypeWrong {
        return nil, errors.New("请使用 Refresh Token 刷新")
    }

    return pair, nil
}

pair, err := h.authSvc.RefreshAccessToken(req.RefreshToken)
```

对应关系：

```text
return nil, errors.New("...")
       ↓            ↓
     pair          err
```

因此：

```go
if err != nil {
    // err.Error() 就是 Service 中 errors.New(...) 保存的文字
    response.Fail(c, response.ErrTokenExpired, err.Error())
    return
}
```

### 4.4 对象是否初始化

```go
if SMSSvc == nil {
    return errors.New("短信服务未初始化")
}
```

这里不是说短信发送出错，而是说 `SMSSvc` 这个指针根本没有指向任何短信服务对象。继续调用 `SMSSvc.SendCode()` 会发生 panic，所以要提前返回。

---

## 5. 统一响应和错误处理

项目统一响应结构：

```go
type Response struct {
    Code    int    `json:"code"`     // 0 成功，非 0 业务失败
    Msg     string `json:"msg"`      // 给前端看的信息
    TraceID string `json:"trace_id"` // 排查日志使用
    Data    any    `json:"data"`     // 真正的响应数据
}
```

成功：

```go
response.Success(c, gin.H{
    "user_id":  user.ID,
    "username": user.Username,
})
```

响应示例：

```json
{
  "code": 0,
  "msg": "success",
  "trace_id": "...",
  "data": {
    "user_id": 1,
    "username": "admin"
  }
}
```

失败：

```go
response.Fail(c, response.ErrUnauthorized, "用户名或密码错误")
```

这个项目大多数成功和业务失败都返回 HTTP 200，前端通过 JSON 中的 `code` 判断结果。限流返回 HTTP 429、OPTIONS 返回 204、OAuth 跳转返回 302。

统一 JSON 绑定：

```go
func ShouldBindJSON(c *gin.Context, obj any) bool {
    if err := c.ShouldBindJSON(obj); err != nil {
        Fail(c, ErrParamInvalid, utils.TranslateValidationError(err))
        return false // 已经给前端响应过错误
    }
    return true
}
```

Handler 使用方式：

```go
if !response.ShouldBindJSON(c, &req) {
    return // 必须 return，不能再继续执行 Service
}
```

---

## 6. 注册业务

### 6.1 接口和参数

```text
POST /api/v1/auth/register
```

```go
type RegisterReq struct {
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required"`
    Nickname string `json:"nickname"`
    Email    string `json:"email"`
    Phone    string `json:"phone"`
}
```

### 6.2 Handler 当前实现

```go
func (h *AuthHandler) Register(c *gin.Context) {
    // 第一步：读取当前热更新配置，判断是否允许注册
    if !config.GlobalManager.Get().SystemSwitches.EnableRegister {
        response.Fail(c, 403, "系统当前已暂停新用户注册，请联系管理员")
        return
    }

    // 第二步：把前端 JSON 绑定到 req
    var req RegisterReq
    if !response.ShouldBindJSON(c, &req) {
        return
    }

    // 第三步：把业务交给 Service
    user, err := h.authSvc.Register(
        req.Username,
        req.Password,
        req.Nickname,
        req.Email,
        req.Phone,
    )
    if err != nil {
        response.Fail(c, response.ErrUserAlreadyExists, err.Error())
        return
    }

    // 第四步：只返回安全字段，不返回密码哈希
    response.Success(c, gin.H{
        "user_id":  user.ID,
        "username": user.Username,
    })
}
```

### 6.3 Service 当前实现

```go
func (s *AuthService) Register(
    username string,
    password string,
    nickname string,
    email string,
    phone string,
) (*model.User, error) {
    // 查到了用户，err == nil，所以用户名已经存在
    if _, err := s.userRepo.FindByUsername(username); err == nil {
        return nil, errors.New("用户名已存在")
    }

    // 不能保存明文密码，先使用 bcrypt 哈希
    hashedPwd, err := utils.HashPassword(password)
    if err != nil {
        return nil, err
    }

    user := &model.User{
        Username: username,
        Password: hashedPwd,
        Nickname: nickname,
        Email:    email,
        Phone:    phone,
        Status:   1, // 新用户默认启用
    }

    if err := s.userRepo.Create(user); err != nil {
        return nil, err
    }

    // GORM Create 成功后，会把数据库生成的 ID 回填到 user.ID
    return user, nil
}
```

### 6.4 注册的业务判断顺序

```text
注册开关是否打开
  ↓ 否：拒绝注册
JSON 参数是否完整
  ↓ 否：参数错误
用户名是否已经存在
  ↓ 是：用户名已存在
密码 bcrypt 加密
  ↓ 失败：返回错误
写入 sys_user
  ↓ 失败：返回数据库错误
返回用户 ID 和用户名
```

### 6.5 当前实现的缺口

普通注册只创建 `sys_user`，没有同时：

- 创建 `user_auths` 中的 password 登录渠道；
- 分配 `user` 角色到 `user_roles`；
- 添加 Casbin 的 `g, userID, user` 规则。

因此新用户虽然可以用 `sys_user.password` 登录，但登录后可能因为没有 Casbin 角色而访问受保护接口时得到 403。

推荐把“创建用户、创建认证记录、分配角色”放到一个数据库事务中，示例见第 23 节。

---

## 7. 用户名密码登录

### 7.1 接口和参数

```text
POST /api/v1/auth/login
```

```go
type LoginReq struct {
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required"`
}
```

### 7.2 Handler

```go
func (h *AuthHandler) Login(c *gin.Context) {
    var req LoginReq
    if !response.ShouldBindJSON(c, &req) {
        return
    }

    pair, user, err := h.authSvc.Login(req.Username, req.Password)
    if err != nil {
        response.Fail(c, response.ErrUnauthorized, err.Error())
        return
    }

    response.Success(c, gin.H{
        "user_id":       user.ID,
        "username":      user.Username,
        "access_token":  pair.AccessToken,
        "refresh_token": pair.RefreshToken,
        "expires_in":    pair.ExpiresIn,
        "token_type":    "Bearer",
    })
}
```

`pair, user, err` 的含义：

```text
pair  登录成功后生成的一对 Access Token 和 Refresh Token
user  数据库中查询到的用户
err   登录任意步骤失败时的错误原因
```

### 7.3 Service

```go
func (s *AuthService) Login(
    username string,
    password string,
) (*token.TokenPair, *model.User, error) {
    // 1. 根据用户名查询用户
    user, err := s.userRepo.FindByUsername(username)
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            // 不告诉前端到底是用户名错还是密码错，防止枚举用户名
            return nil, nil, errors.New("用户名或密码错误")
        }
        return nil, nil, err
    }

    // 2. 禁用用户不能登录
    if user.Status == 0 {
        return nil, nil, errors.New("账户已被禁用")
    }

    // 3. 用 bcrypt 比较明文密码和数据库哈希
    if !utils.CheckPasswordHash(password, user.Password) {
        return nil, nil, errors.New("用户名或密码错误")
    }

    // 4. 查询角色编码，放进 JWT Claims
    roles, _ := s.userRepo.GetUserRoles(user.ID)
    var roleNames []string
    for _, role := range roles {
        roleNames = append(roleNames, role.Code)
    }

    // 5. 生成 Token 对
    pair, err := token.GenerateTokenPair(
        user.ID,
        user.Username,
        roleNames,
    )
    if err != nil {
        return nil, nil, fmt.Errorf("生成 Token 失败: %w", err)
    }

    return pair, user, nil
}
```

登录的判断顺序一定要记住：

```text
用户是否存在 → 用户是否启用 → 密码是否正确 → 查询角色 → 生成 Token
```

打印登录参数时可以这样写：

```go
zap.L().Info("用户尝试登录",
    zap.String("username", req.Username),
    zap.String("trace_id", c.GetString("trace_id")),
)

// 绝对不要这样打印：
// zap.String("password", req.Password)
// zap.String("access_token", pair.AccessToken)
```

---

## 8. JWT 认证和 Casbin 鉴权

### 8.1 为什么登录后还要经过中间件

登录成功只说明用户名和密码正确。访问 `/api/v1/users` 时还要判断：

1. 是否携带 Token；
2. Token 是否有效；
3. 是否误用了 Refresh Token；
4. Token 是否已经退出登录；
5. 当前用户有没有访问该 URL 和 HTTP 方法的权限。

### 8.2 JWT 中间件核心代码

```go
func JWTAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Authorization: Bearer eyJhbGci...
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
            // Abort 表示终止后面的 Handler 链
            c.AbortWithStatusJSON(http.StatusOK, response.Response{
                Code: response.ErrUnauthorized,
                Msg:  "未登录或 Token 格式错误",
            })
            return
        }

        tokenString := authHeader[7:] // 删除前面的 "Bearer "
        claims, err := token.ParseToken(tokenString)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusOK, response.Response{
                Code: response.ErrTokenExpired,
                Msg:  "Token 已过期或无效",
            })
            return
        }

        // Refresh Token 只能刷新，不能直接访问业务 API
        if claims.TokenType != "access" {
            c.AbortWithStatusJSON(http.StatusOK, response.Response{
                Code: response.ErrUnauthorized,
                Msg:  "请使用 Access Token 访问",
            })
            return
        }

        // Redis 中存在这个 JTI，说明用户已经退出登录
        if token.IsBlacklisted(claims.JTIShort) {
            c.AbortWithStatusJSON(http.StatusOK, response.Response{
                Code: response.ErrTokenExpired,
                Msg:  "Token 已被注销",
            })
            return
        }

        // 把 Token 中的数据写入 Gin Context，给 Handler 使用
        c.Set("current_user_id", claims.UserID)
        c.Set("current_username", claims.Username)
        c.Set("current_user_roles", claims.Roles)
        c.Set("current_token_jti", claims.JTIShort)

        // 使用用户 ID、请求路径、HTTP 方法进行 Casbin 判断
        userID := fmt.Sprintf("%d", claims.UserID)
        path := c.Request.URL.Path
        method := c.Request.Method

        allowed, _ := casbin.Enforcer.Enforce(userID, path, method)

        // /users/15 转成 /users/:id 后再匹配一次
        normalizedPath := numericPathRegex.ReplaceAllString(path, "/:id")
        if !allowed && normalizedPath != path {
            allowed, _ = casbin.Enforcer.Enforce(
                userID,
                normalizedPath,
                method,
            )
        }

        if !allowed {
            c.AbortWithStatusJSON(http.StatusOK, response.Response{
                Code: response.ErrForbidden,
                Msg:  "权限不足，拒绝访问",
            })
            return
        }

        c.Next() // 认证和鉴权都成功，继续执行 Handler
    }
}
```

### 8.3 Casbin 的两类规则

用户和角色的关系：

```text
g, 15, editor
```

表示用户 ID 15 拥有 editor 角色。

角色和接口权限的关系：

```text
p, editor, /api/v1/users, GET
p, editor, /api/v1/users/:id, PUT
```

表示 editor 可以查询用户列表、修改指定用户。

完整判断过程：

```text
用户 15
  ↓ 查 g 规则
editor 角色
  ↓ 查 p 规则
/api/v1/users/:id + PUT
  ↓
允许或拒绝
```

管理员策略：

```go
casbin.AddRoleForUser(userIDString, "admin")
casbin.AddPolicy("admin", "*", "*")
```

当前真正运行的是 `JWTAuth()` 内以用户 ID 为 subject 的鉴权。项目另有 `CasbinCheck()`，但没有挂到路由中。

---

## 9. Token 刷新与退出登录

### 9.1 双 Token

```go
const (
    AccessTokenTTL  = 15 * time.Minute
    RefreshTokenTTL = 7 * 24 * time.Hour
)
```

```text
Access Token   有效期短，每次访问 API 都携带
Refresh Token  有效期长，只用于换取新的 Token 对
```

### 9.2 刷新 Token

请求参数：

```go
type RefreshReq struct {
    RefreshToken string `json:"refresh_token" binding:"required"`
}
```

Service：

```go
func (s *AuthService) RefreshAccessToken(
    refreshTokenString string,
) (*token.TokenPair, error) {
    // 1. 检查签名和过期时间
    claims, err := token.ParseToken(refreshTokenString)
    if err != nil {
        return nil, errors.New("Refresh Token 无效或已过期")
    }

    // 2. 防止拿 Access Token 冒充 Refresh Token
    if claims.TokenType != "refresh" {
        return nil, errors.New("请使用 Refresh Token 刷新")
    }

    // 3. 检查黑名单
    if token.IsBlacklisted(claims.JTIShort) {
        return nil, errors.New("Refresh Token 已被注销")
    }

    // 4. 每次刷新重新查用户状态
    user, err := s.userRepo.FindByID(claims.UserID)
    if err != nil {
        return nil, errors.New("用户不存在")
    }
    if user.Status == 0 {
        return nil, errors.New("账户已被禁用")
    }

    // 5. 重新查询当前角色，避免一直使用旧角色
    roles, _ := s.userRepo.GetUserRoles(user.ID)
    var roleNames []string
    for _, role := range roles {
        roleNames = append(roleNames, role.Code)
    }

    // 6. 返回一对全新的 Token
    return token.GenerateTokenPair(user.ID, user.Username, roleNames)
}
```

### 9.3 退出登录

```go
func (h *AuthHandler) Logout(c *gin.Context) {
    // current_token_jti 是 JWTAuth 中间件写入的
    jti, exists := c.Get("current_token_jti")
    if !exists {
        response.Fail(c, response.ErrUnauthorized, "未登录")
        return
    }

    jtiString, ok := jti.(string)
    if !ok {
        response.Fail(c, response.ErrSystemError, "Token 上下文异常")
        return
    }

    // 把当前 Access Token 的唯一编号放入 Redis 黑名单
    if err := token.BlacklistToken(jtiString, token.AccessTokenTTL); err != nil {
        response.Fail(c, response.ErrSystemError, "退出登录失败")
        return
    }

    response.Success(c, gin.H{})
}
```

当前源码忽略了 `BlacklistToken` 返回的错误，上面示例补上了推荐的错误判断。

当前退出只注销 Access Token，没有注销 Refresh Token。前端会删除本地两个 Token，但服务端旧 Refresh Token 在过期前仍可能刷新。

---

## 10. 短信验证码与短信登录

### 10.1 当前短信服务

开发模式验证码固定为 `123456`，保存在后端进程内存中：

```go
type SMSService struct {
    mu      sync.Mutex
    codes   map[string]codeEntry // 手机号 → 验证码和过期时间
    rateMap map[string]time.Time // 手机号 → 上次发送时间
}

type codeEntry struct {
    Code      string
    ExpiresAt time.Time
}
```

发送验证码：

```go
func (s *SMSService) SendCode(phone string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // 同一个手机号 60 秒内不能重复发送
    if last, exists := s.rateMap[phone];
        exists && time.Since(last) < 60*time.Second {
        return errors.New("验证码发送过于频繁，请 60 秒后再试")
    }

    code := "123456" // 仅开发环境
    s.codes[phone] = codeEntry{
        Code:      code,
        ExpiresAt: time.Now().Add(5 * time.Minute),
    }
    s.rateMap[phone] = time.Now()

    zap.L().Info("SMS 验证码（开发模式）",
        zap.String("phone", phone),
        zap.String("code", code),
    )
    return nil
}
```

校验验证码：

```go
func (s *SMSService) VerifyCode(phone, code string) bool {
    s.mu.Lock()
    defer s.mu.Unlock()

    entry, exists := s.codes[phone]
    if !exists {
        return false
    }

    if time.Now().After(entry.ExpiresAt) {
        delete(s.codes, phone)
        return false
    }

    if entry.Code != code {
        return false
    }

    // 验证成功立即删除，验证码只能使用一次
    delete(s.codes, phone)
    return true
}
```

### 10.2 短信登录的两种业务分支

```go
func (s *AuthService) LoginBySMS(
    phone string,
    code string,
) (*token.TokenPair, *model.User, error) {
    if SMSSvc == nil {
        return nil, nil, errors.New("短信服务未初始化")
    }

    if !SMSSvc.VerifyCode(phone, code) {
        return nil, nil, errors.New("验证码错误或已过期")
    }

    auth, err := s.userAuthRepo.FindByIdentity("phone", phone)
    if err != nil {
        if !errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, nil, err
        }

        // 分支一：手机号从未登录过，自动注册
        user := &model.User{
            Username: "u_" + phone,
            Nickname: phone,
            Phone:    phone,
            Status:   1,
        }

        // 如果自动用户名重复，追加 _1、_2……
        baseName := user.Username
        for i := 1; ; i++ {
            _, findErr := s.userRepo.FindByUsername(user.Username)
            if errors.Is(findErr, gorm.ErrRecordNotFound) {
                break
            }
            if findErr != nil {
                return nil, nil, findErr
            }
            user.Username = fmt.Sprintf("%s_%d", baseName, i)
        }

        if err := s.userRepo.Create(user); err != nil {
            return nil, nil, fmt.Errorf("自动注册失败: %w", err)
        }

        // 建立手机号与用户的登录关系
        phoneAuth := &model.UserAuth{
            UserID:       user.ID,
            IdentityType: "phone",
            Identifier:   phone,
        }
        if err := s.userAuthRepo.Create(phoneAuth); err != nil {
            return nil, nil, err
        }

        // 给新用户分配 user 角色
        userRole, err := s.roleRepo.FindByCode("user")
        if err == nil {
            _ = s.userRepo.AssignRoles(user.ID, []uint64{userRole.ID})
            _, _ = casbin.AddRoleForUser(
                fmt.Sprintf("%d", user.ID),
                "user",
            )
            _ = casbin.LoadPolicy()
        }

        pair, err := token.GenerateTokenPair(
            user.ID,
            user.Username,
            []string{"user"},
        )
        return pair, user, err
    }

    // 分支二：手机号已有绑定，找到原用户并登录
    user, err := s.userRepo.FindByID(auth.UserID)
    if err != nil {
        return nil, nil, errors.New("用户不存在")
    }
    if user.Status == 0 {
        return nil, nil, errors.New("账户已被禁用")
    }

    roles, err := s.userRepo.GetUserRoles(user.ID)
    if err != nil {
        return nil, nil, err
    }
    roleNames := make([]string, 0, len(roles))
    for _, role := range roles {
        roleNames = append(roleNames, role.Code)
    }

    pair, err := token.GenerateTokenPair(
        user.ID,
        user.Username,
        roleNames,
    )
    return pair, user, err
}
```

上面的代码在“当前业务思路”上与项目一致，同时把项目里部分被忽略的错误补全了。

生产环境要把验证码改成随机数，并放入 Redis，这样多实例部署时每台后端都能校验同一份验证码。

---

## 11. GitHub OAuth 登录

### 11.1 完整流程

```text
用户点击 GitHub 登录
  ↓
GET /auth/oauth/github
  ↓
后端 302 跳转到 GitHub 授权页
  ↓
用户同意授权
  ↓
GitHub 回调 /auth/oauth/github/callback?code=xxx
  ↓
后端用 code 换 GitHub Access Token
  ↓
调用 GitHub /user 获取用户资料
  ↓
查询 user_auths 中是否已有 github 身份
  ├── 没有：自动创建本站用户、认证记录、user 角色
  └── 已有：找到原用户并登录
  ↓
生成本站 JWT
  ↓
302 跳回普通用户前端
```

生成授权地址：

```go
func (s *AuthService) GetGitHubAuthURL() (string, error) {
    if GitHubOAuth == nil {
        return "", errors.New("GitHub OAuth 未配置")
    }

    // 当前项目使用固定 state "keystone"
    return GitHubOAuth.GetAuthURL("keystone"), nil
}
```

回调 Handler：

```go
func (h *AuthHandler) GitHubCallback(c *gin.Context) {
    code := c.Query("code")
    if code == "" {
        response.Fail(c, response.ErrParamInvalid, "缺少授权码")
        return
    }

    pair, user, err := h.authSvc.LoginByGitHub(code)
    if err != nil {
        response.Fail(c, response.ErrUnauthorized, err.Error())
        return
    }

    // Token 放在 URL fragment（# 后面），通常不会进入服务端访问日志
    redirectURL := fmt.Sprintf(
        "%s/auth/oauth/callback#access_token=%s&refresh_token=%s&expires_in=%d&user_id=%d&username=%s",
        h.authSvc.GetFrontendURL(),
        pair.AccessToken,
        pair.RefreshToken,
        pair.ExpiresIn,
        user.ID,
        url.QueryEscape(user.Username),
    )
    c.Redirect(http.StatusFound, redirectURL)
}
```

OAuth Service 的核心分支：

```go
// 1. code 换 GitHub Token
accessToken, err := GitHubOAuth.ExchangeCode(code)
if err != nil {
    return nil, nil, fmt.Errorf("GitHub 授权失败: %w", err)
}

// 2. GitHub Token 换 GitHub 用户资料
githubUser, err := GitHubOAuth.GetUser(accessToken)
if err != nil {
    return nil, nil, fmt.Errorf("获取 GitHub 用户信息失败: %w", err)
}

// 3. GitHub ID 才是稳定身份，用户名可能被修改
identifier := fmt.Sprintf("github_%d", githubUser.ID)
auth, err := s.userAuthRepo.FindByIdentity("github", identifier)

// 4. ErrRecordNotFound 表示首次 GitHub 登录，需要自动注册
if errors.Is(err, gorm.ErrRecordNotFound) {
    // 创建 sys_user
    // 创建 user_auths
    // 分配 user 角色
    // 生成本站 TokenPair
}
```

安全注意：当前 `state` 是固定值，没有完整防 CSRF 校验；GitHub Access Token 被保存到了 `user_auths.credential`。生产环境应生成一次性 state 并校验，第三方 Token 应加密存储或在不需要长期调用 GitHub 时不保存。

---

## 12. 登录渠道绑定、解绑

`user_auths` 表允许一个用户拥有多种登录方式：

```go
type UserAuth struct {
    BaseModel
    UserID       uint64 `json:"user_id"`
    IdentityType string `json:"identity_type"` // password/phone/github
    Identifier   string `json:"identifier"`    // 用户名/手机号/GitHub ID
    Credential   string `json:"-"`             // 密码哈希/OAuth Token
}
```

### 12.1 查询登录渠道

```go
func (s *AuthService) GetUserChannels(
    userID uint64,
) ([]model.UserAuth, error) {
    return s.userAuthRepo.ListByUserID(userID)
}
```

`Credential` 使用 `json:"-"`，返回 JSON 时不会泄露凭证。

### 12.2 绑定手机号

```go
func (s *AuthService) BindPhone(
    userID uint64,
    phone string,
    code string,
) error {
    if SMSSvc == nil {
        return errors.New("短信服务未初始化")
    }
    if !SMSSvc.VerifyCode(phone, code) {
        return errors.New("验证码错误或已过期")
    }

    // 手机号是全局登录标识，不能被两个账户绑定
    _, err := s.userAuthRepo.FindByIdentity("phone", phone)
    if err == nil {
        return errors.New("该手机号已绑定其他账户")
    }
    if !errors.Is(err, gorm.ErrRecordNotFound) {
        return err
    }

    auth := &model.UserAuth{
        UserID:       userID,
        IdentityType: "phone",
        Identifier:   phone,
    }
    return s.userAuthRepo.Create(auth)
}
```

当前实现只写 `user_auths`，不会同步修改 `sys_user.phone`。要先明确产品规则：资料手机号和登录手机号是否必须一致。

### 12.3 解绑渠道

```go
func (s *AuthService) UnbindChannel(
    userID uint64,
    identityType string,
) error {
    channels, err := s.userAuthRepo.ListByUserID(userID)
    if err != nil {
        return err
    }

    // 防止用户把最后一个登录方式也删掉
    if len(channels) <= 1 {
        return errors.New("至少保留一种登录方式")
    }

    return s.userAuthRepo.DeleteByType(userID, identityType)
}
```

推荐再判断被解绑的类型是否真的存在，并检查 `RowsAffected`，否则删除 0 行也会返回 `nil`。

---

## 13. 找回和重置密码

找回密码分两步，不能把“发验证码”和“改密码”合成一个无校验接口。

### 13.1 发送找回验证码

```go
func (s *AuthService) ForgotPassword(phone string) error {
    if SMSSvc == nil {
        return errors.New("短信服务未初始化")
    }

    // 只有已经绑定手机号的用户才能通过手机找回
    _, err := s.userAuthRepo.FindByIdentity("phone", phone)
    if err != nil {
        return errors.New("该手机号未注册")
    }

    return SMSSvc.SendCode(phone)
}
```

### 13.2 校验验证码并重置密码

```go
func (s *AuthService) ResetPassword(
    phone string,
    code string,
    newPassword string,
) error {
    if SMSSvc == nil {
        return errors.New("短信服务未初始化")
    }
    if !SMSSvc.VerifyCode(phone, code) {
        return errors.New("验证码错误或已过期")
    }

    auth, err := s.userAuthRepo.FindByIdentity("phone", phone)
    if err != nil {
        return errors.New("该手机号未绑定任何账户")
    }

    hashedPassword, err := utils.HashPassword(newPassword)
    if err != nil {
        return err
    }

    return s.userRepo.UpdatePassword(auth.UserID, hashedPassword)
}
```

推荐增强：重置成功后注销该用户全部旧 Token，避免已泄露的 Token 继续有效。

---

## 14. 当前用户资料与头像

### 14.1 获取当前用户

```go
func (h *UserHandler) GetCurrentUser(c *gin.Context) {
    value, exists := c.Get("current_user_id")
    if !exists {
        response.Fail(c, response.ErrUnauthorized, "未登录")
        return
    }

    userID, ok := value.(uint64)
    if !ok {
        response.Fail(c, response.ErrSystemError, "用户上下文异常")
        return
    }

    user, err := h.userSvc.GetByID(userID)
    if err != nil {
        response.Fail(c, response.ErrUserNotFound, "用户不存在")
        return
    }

    roles, err := h.userSvc.GetRoles(userID)
    if err != nil {
        response.Fail(c, response.ErrSystemError, "查询角色失败")
        return
    }

    response.Success(c, gin.H{
        "user":  user,
        "roles": roles,
    })
}
```

### 14.2 修改资料

```go
type UpdateProfileReq struct {
    Nickname string `json:"nickname"`
    Email    string `json:"email"`
    Phone    string `json:"phone"`
}

func (s *UserService) UpdateProfile(
    userID uint64,
    nickname string,
    email string,
    phone string,
) error {
    user, err := s.userRepo.FindByID(userID)
    if err != nil {
        return err
    }

    // 当前代码只更新非空字段
    if nickname != "" {
        user.Nickname = nickname
    }
    if email != "" {
        user.Email = email
    }
    if phone != "" {
        user.Phone = phone
    }

    return s.userRepo.Update(user)
}
```

这种写法无法把昵称、邮箱、手机号主动清空。需要区分“未传字段”和“传入空字符串”时，DTO 应使用指针：

```go
type UpdateProfileReq struct {
    Nickname *string `json:"nickname"`
    Email    *string `json:"email"`
    Phone    *string `json:"phone"`
}

if req.Nickname != nil {
    user.Nickname = *req.Nickname
}
```

### 14.3 上传头像

```go
func (h *UserHandler) UploadAvatar(c *gin.Context) {
    userIDValue, _ := c.Get("current_user_id")
    userID, _ := userIDValue.(uint64)

    file, err := c.FormFile("avatar")
    if err != nil {
        response.Fail(c, response.ErrParamInvalid, "请选择头像文件")
        return
    }

    // 例如 15_1786000000.png
    filename := fmt.Sprintf(
        "%d_%d%s",
        userID,
        time.Now().Unix(),
        filepath.Ext(file.Filename),
    )
    savePath := filepath.Join("uploads", "avatars", filename)

    if err := c.SaveUploadedFile(file, savePath); err != nil {
        response.Fail(c, response.ErrSystemError, "头像上传失败")
        return
    }

    avatarURL := "/uploads/avatars/" + filename
    if err := h.userSvc.UpdateAvatar(userID, avatarURL); err != nil {
        response.Fail(c, response.ErrSystemError, "更新头像失败")
        return
    }

    response.Success(c, gin.H{"avatar_url": avatarURL})
}
```

生产环境还应校验文件大小、MIME 类型、扩展名，创建目录，删除旧头像，并处理“文件保存成功但数据库更新失败”的补偿逻辑。

---

## 15. 用户管理业务

### 15.1 数据模型

```go
type User struct {
    BaseModel
    Username string `gorm:"size:50;not null;uniqueIndex" json:"username"`
    Password string `gorm:"size:255;not null" json:"-"`
    Nickname string `gorm:"size:50" json:"nickname"`
    Email    string `gorm:"size:100" json:"email"`
    Phone    string `gorm:"size:20" json:"phone"`
    Avatar   string `gorm:"size:255" json:"avatar"`
    Status   int8   `gorm:"default:1" json:"status"`
    IsAdmin  bool   `gorm:"default:false" json:"is_admin"`
    Roles    []Role `gorm:"many2many:user_roles" json:"roles,omitempty"`
}
```

`Password` 的 `json:"-"` 表示：

- 返回用户 JSON 时不输出密码；
- 直接用 `model.User` 绑定 JSON 时也不能通过 JSON 给 Password 赋值。

### 15.2 用户管理接口

```text
GET    /api/v1/users                 分页查询
GET    /api/v1/users/:id             查询详情
POST   /api/v1/users                 管理员创建用户
PUT    /api/v1/users/:id             修改用户
DELETE /api/v1/users/:id             删除用户
PUT    /api/v1/users/:id/status      启用/禁用
PUT    /api/v1/users/:id/password    重置密码
GET    /api/v1/users/:id/roles       查询用户角色
PUT    /api/v1/users/:id/roles       分配角色
```

### 15.3 管理员创建用户

当前 Service：

```go
func (s *UserService) Create(user *model.User) error {
    // 当前项目固定使用默认密码 123456
    hashedPassword, err := utils.HashPassword("123456")
    if err != nil {
        return err
    }

    user.Password = hashedPassword
    user.Status = 1
    return s.userRepo.Create(user)
}
```

推荐定义专用 DTO，不直接绑定数据库 Model：

```go
type CreateUserReq struct {
    Username string `json:"username" binding:"required,min=3,max=50"`
    Password string `json:"password" binding:"required,min=6,max=72"`
    Nickname string `json:"nickname" binding:"max=50"`
    Email    string `json:"email" binding:"omitempty,email"`
    Phone    string `json:"phone"`
    RoleIDs  []uint64 `json:"role_ids"`
}
```

这样前端无法偷偷传入 `is_admin: true`、自定义 `id` 或时间字段。

### 15.4 修改状态

```go
type UpdateStatusReq struct {
    Status int8 `json:"status" binding:"oneof=0 1"`
}

func (s *UserService) UpdateStatus(userID uint64, status int8) error {
    return s.userRepo.UpdateStatus(userID, status)
}
```

禁用用户后的当前行为：

- 下次密码登录会被拒绝；
- 下次刷新 Token 会被拒绝；
- 已经签发的 Access Token 不会立即失效，因为 JWT 中间件不重新查询用户状态。

### 15.5 管理员重置密码

```go
func (s *UserService) UpdatePassword(
    userID uint64,
    plainPassword string,
) error {
    hashedPassword, err := utils.HashPassword(plainPassword)
    if err != nil {
        return err
    }
    return s.userRepo.UpdatePassword(userID, hashedPassword)
}
```

推荐重置后注销旧 Token。

### 15.6 删除用户

当前通用删除：

```go
func (r *BaseRepository[T]) Delete(id uint64) error {
    var entity T
    return r.db.Delete(&entity, id).Error
}
```

`BaseModel` 没有 `gorm.DeletedAt`，所以这是物理删除。当前不会主动清理：

- `user_roles`；
- `user_auths`；
- Casbin 中用户的 `g` 规则；
- 用户上传的头像文件。

正式业务应该使用事务做级联清理，或者增加外键约束和软删除。

### 15.7 给用户分配角色

数据库关联使用替换模式：

```go
func (r *UserRepository) AssignRoles(
    userID uint64,
    roleIDs []uint64,
) error {
    user := model.User{
        BaseModel: model.BaseModel{ID: userID},
    }

    roles := make([]model.Role, 0, len(roleIDs))
    for _, roleID := range roleIDs {
        roles = append(roles, model.Role{
            BaseModel: model.BaseModel{ID: roleID},
        })
    }

    // Replace：先移除旧关联，再保存新的关联
    return r.db.Model(&user).Association("Roles").Replace(roles)
}
```

同步 Casbin 的推荐写法：

```go
func (s *UserService) AssignRoles(
    userID uint64,
    roleIDs []uint64,
) error {
    userIDString := strconv.FormatUint(userID, 10)

    // 注意：清除角色关系要用 DeleteRolesForUser，
    // 不是 DeletePermissionsForUser。
    if _, err := casbin.Enforcer.DeleteRolesForUser(userIDString); err != nil {
        return err
    }

    if err := s.userRepo.AssignRoles(userID, roleIDs); err != nil {
        return err
    }

    for _, roleID := range roleIDs {
        role, err := s.roleRepo.FindByID(roleID)
        if err != nil {
            return err
        }
        if role.Status != 1 {
            return fmt.Errorf("角色 %s 已禁用", role.Name)
        }
        if _, err := casbin.AddRoleForUser(userIDString, role.Code); err != nil {
            return err
        }
    }

    return casbin.LoadPolicy()
}
```

当前项目在这里使用了 `DeletePermissionsForUser(userID)`，它删除的是 `p` 权限规则，不是 `g` 角色关系，旧角色可能残留在 Casbin 中。

---

## 16. 角色管理业务

### 16.1 数据模型

```go
type Role struct {
    BaseModel
    Name   string `json:"name"`
    Code   string `gorm:"uniqueIndex" json:"code"`
    Desc   string `json:"desc"`
    Status int8   `json:"status"`
    Menus  []Menu `gorm:"many2many:role_menus" json:"menus,omitempty"`
}
```

角色 Code 很重要，因为 Casbin 规则保存的是 Code，不是角色 ID：

```text
sys_role:     id=2, code=editor
casbin_rule:  p, editor, /api/v1/users, GET
```

### 16.2 角色接口

```text
GET    /api/v1/roles
GET    /api/v1/roles/:id
POST   /api/v1/roles
PUT    /api/v1/roles/:id
DELETE /api/v1/roles/:id
PUT    /api/v1/roles/:id/status
GET    /api/v1/roles/:id/menus
PUT    /api/v1/roles/:id/menus
PUT    /api/v1/roles/:id/permissions
```

### 16.3 创建角色

```go
func (s *RoleService) Create(role *model.Role) error {
    role.Status = 1
    return s.roleRepo.Create(role)
}
```

数据库对 `code` 有唯一索引，重复创建会返回数据库错误。

### 16.4 给角色分配菜单

```go
func (s *RoleService) AssignMenus(
    roleID uint64,
    menuIDs []uint64,
) error {
    // GORM Association Replace 会替换 role_menus 中的旧数据
    return s.roleRepo.AssignRoleMenus(roleID, menuIDs)
}
```

菜单决定前端侧边栏显示什么，不等于后端 API 权限。

### 16.5 给角色分配 API 权限

参数：

```go
type PermItem struct {
    Path   string `json:"path"`
    Method string `json:"method"`
}
```

业务：

```go
func (s *RoleService) AssignPermissions(
    roleID uint64,
    permissions []PermItem,
) error {
    role, err := s.roleRepo.FindByID(roleID)
    if err != nil {
        return err
    }

    // 删除该角色旧的 p 规则
    if _, err := casbinpkg.DeletePermissionsForUser(role.Code); err != nil {
        return err
    }

    // 添加新的 p 规则
    for _, permission := range permissions {
        if _, err := casbinpkg.AddPolicy(
            role.Code,
            permission.Path,
            strings.ToUpper(permission.Method),
        ); err != nil {
            return err
        }
    }

    return casbinpkg.LoadPolicy()
}
```

请求示例：

```json
{
  "permissions": [
    {"path": "/api/v1/users", "method": "GET"},
    {"path": "/api/v1/users/:id", "method": "PUT"}
  ]
}
```

### 16.6 修改角色 Code 的影响

如果把 `editor` 改为 `auditor`，`sys_role.code` 变了，但原来的 Casbin 规则仍然可能是：

```text
g, 15, editor
p, editor, /api/v1/users, GET
```

因此修改角色 Code 时必须同步迁移 Casbin 的 `g` 和 `p` 规则，或者规定角色 Code 创建后不能修改。

### 16.7 禁用角色

当前只修改 `sys_role.status`。JWTAuth 的 Casbin 判断不检查角色状态，所以仅把状态改成 0 不会立即撤销实际权限。推荐禁用时同步删除/停用 Casbin 关系，或在鉴权时检查角色状态。

---

## 17. 菜单管理业务

### 17.1 菜单模型

```go
type Menu struct {
    BaseModel
    ParentID  uint64  `json:"parent_id"`
    Name      string  `json:"name"`
    Path      string  `json:"path"`
    Component string  `json:"component"`
    Icon      string  `json:"icon"`
    Sort      int     `json:"sort"`
    Type      int     `json:"type"`   // 1目录 2菜单 3按钮
    Perms     string  `json:"perms"`  // 前端权限标识
    Status    int8    `json:"status"`
    Hidden    bool    `json:"hidden"`
    Children  []*Menu `gorm:"-" json:"children,omitempty"`
}
```

`Children` 的 `gorm:"-"` 表示它不是数据库字段，只在返回菜单树时临时组装。

### 17.2 菜单接口

```text
GET    /api/v1/menus
GET    /api/v1/menus/tree
GET    /api/v1/menus/:id
POST   /api/v1/menus
PUT    /api/v1/menus/:id
DELETE /api/v1/menus/:id
PUT    /api/v1/menus/:id/status
GET    /api/v1/user/menus
```

### 17.3 查询当前用户菜单

Repository 的 SQL 思路：

```go
func (r *MenuRepository) GetUserMenus(
    userID uint64,
) ([]model.Menu, error) {
    var menus []model.Menu

    err := r.db.Raw(`
        SELECT DISTINCT m.*
        FROM sys_menu m
        INNER JOIN role_menus rm ON m.id = rm.menu_id
        INNER JOIN user_roles ur ON rm.role_id = ur.role_id
        WHERE ur.user_id = ?
          AND m.status = 1
        ORDER BY m.sort ASC
    `, userID).Scan(&menus).Error

    return menus, err
}
```

关系链：

```text
当前用户
  ↓ user_roles
拥有的角色
  ↓ role_menus
角色拥有的菜单
  ↓ ParentID
组装为菜单树
```

Service：

```go
func (s *MenuService) GetUserMenus(
    userID uint64,
) ([]*utils.MenuTreeResp, error) {
    menus, err := s.menuRepo.GetUserMenus(userID)
    if err != nil {
        return nil, err
    }

    // 推荐：没有菜单就返回空数组，不要返回全部菜单
    if len(menus) == 0 {
        return []*utils.MenuTreeResp{}, nil
    }

    return utils.BuildMenuTree(menus), nil
}
```

当前源码在无菜单时回退为全部菜单，这会让没有分配菜单的用户看到所有管理菜单。即使 Casbin 仍可能阻止 API 请求，界面展示也不应该越权。

### 17.4 菜单和权限的关系

```text
菜单分配：控制用户能看到什么入口
Casbin：控制用户能不能真正调用 API
```

前端隐藏按钮不能代替后端鉴权。恶意用户可以不点按钮，直接使用 curl 调 API。

---

## 18. 权限定义与 Casbin 权限的区别

这是这个项目最容易混淆的地方。

### 18.1 `sys_permission`

```go
type Permission struct {
    BaseModel
    Path        string `json:"path"`
    Method      string `json:"method"`
    Description string `json:"description"`
    Status      int8   `json:"status"`
}
```

用途：保存“系统有哪些可选择的 API 权限”，主要给管理后台展示。

### 18.2 `casbin_rule`

```go
type CasbinRule struct {
    Ptype string // p 或 g
    V0    string // 角色 Code 或用户 ID
    V1    string // 路径或角色 Code
    V2    string // HTTP 方法
}
```

用途：JWTAuth 真正进行运行时鉴权。

### 18.3 两者当前没有自动同步

```text
在权限管理页面创建 sys_permission
  ≠ 自动给任何角色授权

删除或禁用 sys_permission
  ≠ 自动删除 Casbin p 规则

调用 PUT /roles/:id/permissions
  = 真正改变角色的 Casbin 权限
```

权限定义列表查询：

```go
type PermissionListReq struct {
    Keyword  string `form:"keyword"`
    Method   string `form:"method"`
    Status   *int8  `form:"status"` // 指针可以区分未传和传 0
    Page     int    `form:"page"`
    PageSize int    `form:"page_size"`
}

func (r *PermissionRepository) ListWithFilter(
    keyword string,
    method string,
    status *int8,
    page int,
    pageSize int,
) ([]model.Permission, int64, error) {
    q := r.db.Model(&model.Permission{})

    if keyword != "" {
        like := "%" + keyword + "%"
        q = q.Where(
            "path LIKE ? OR description LIKE ?",
            like,
            like,
        )
    }
    if method != "" {
        q = q.Where("method = ?", method)
    }
    if status != nil {
        q = q.Where("status = ?", *status)
    }

    // 后续 Count + Offset + Limit + Find
}
```

`backend/config/permissions.yaml` 当前没有加载代码，它只是配置文件草稿，不会自动写入数据库或 Casbin。

---

## 19. 系统监控、健康检查和中间件

### 19.1 全局中间件顺序

```go
r.Use(middleware.Recovery())  // 捕获 panic，避免服务崩溃
r.Use(middleware.Trace())     // 生成 trace_id
r.Use(middleware.Cors())      // 跨域
r.Use(middleware.Logger())    // HTTP 请求日志
r.Use(middleware.Metrics())   // Prometheus 指标
r.Use(middleware.RateLimit()) // 限流
```

中间件的洋葱模型：

```go
func ExampleMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // c.Next() 之前：请求进入时执行
        start := time.Now()

        c.Next() // 执行下一个中间件或 Handler

        // c.Next() 之后：响应返回时执行
        cost := time.Since(start)
        zap.L().Info("请求完成", zap.Duration("cost", cost))
    }
}
```

### 19.2 TraceID

```go
func Trace() gin.HandlerFunc {
    return func(c *gin.Context) {
        traceID := c.GetHeader("X-Trace-Id")
        if traceID == "" {
            traceID = uuid.New().String()
        }

        c.Set("trace_id", traceID)
        c.Header("X-Trace-Id", traceID)

        // 同时传入标准 context，让 GORM SQL 日志也能关联
        ctx := context.WithValue(
            c.Request.Context(),
            database.TraceIDKey,
            traceID,
        )
        c.Request = c.Request.WithContext(ctx)

        c.Next()
    }
}
```

### 19.3 日志

```go
zap.L().Info("创建用户成功",
    zap.Uint64("user_id", user.ID),
    zap.String("username", user.Username),
    zap.String("trace_id", c.GetString("trace_id")),
)

zap.L().Error("创建用户失败",
    zap.String("username", req.Username),
    zap.Error(err),
    zap.String("trace_id", c.GetString("trace_id")),
)
```

建议打印：业务 ID、状态、耗时、错误、TraceID。不要打印：密码、完整 Token、验证码、密钥。

### 19.4 限流

```text
全局：每秒 5000 个令牌，突发容量 10000
单 IP：每秒 50 个令牌，突发容量 100
```

超过限制返回 HTTP 429 和业务码 40029。

### 19.5 健康检查

```text
GET /healthz  只说明 HTTP 服务存活
GET /readyz   执行 SELECT 1，说明数据库可连接
GET /metrics  暴露 Prometheus 指标
```

### 19.6 系统监控

`GET /api/v1/monitor/stats` 返回：

```text
服务运行时长
goroutine 数量
Go 堆内存和系统内存
CPU 核数和 Go 版本
HTTP 请求量
HTTP 5xx 数量
P99 延迟估算
```

因为多数业务失败使用 HTTP 200，当前 `HTTP 5xx` 统计不会把业务码失败计算为错误。

---

## 20. 配置加载和 YAML 关联

### 20.1 YAML

```yaml
system_switches:
  enable_register: true
  maintenance_mode: false
```

### 20.2 Go 结构体

```go
type AppConfig struct {
    App            AppCfg         `mapstructure:"app"`
    JWT            JWTCfg         `mapstructure:"jwt"`
    Database       DatabaseCfg    `mapstructure:"database"`
    Redis          RedisCfg       `mapstructure:"redis"`
    OAuth          OAuthCfg       `mapstructure:"oauth"`
    SystemSwitches SystemSwitches `mapstructure:"system_switches"`
}

type SystemSwitches struct {
    EnableRegister  bool `mapstructure:"enable_register"`
    MaintenanceMode bool `mapstructure:"maintenance_mode"`
}
```

`mapstructure` 标签负责 YAML key 与 Go 字段的对应。

### 20.3 初始化

```go
func InitConfig(bootstrapPath string) {
    v := viper.New()
    v.SetConfigFile(bootstrapPath)
    v.SetConfigType("yaml")

    if err := v.ReadInConfig(); err != nil {
        loadFromBackup()
        return
    }

    var cfg AppConfig
    if err := v.Unmarshal(&cfg, viper.DecodeHook(DecryptHook())); err != nil {
        loadFromBackup()
        return
    }

    // 把局部变量 cfg 的地址保存到全局管理器
    GlobalManager.Update(&cfg)

    // 监听 YAML 文件变化
    v.WatchConfig()
    v.OnConfigChange(func(event fsnotify.Event) {
        var newCfg AppConfig
        if err := v.Unmarshal(&newCfg); err != nil {
            return
        }
        GlobalManager.Update(&newCfg)
    })
}
```

### 20.4 为什么能从任意业务读取

```go
var GlobalManager = &ConfigManager{
    Active: &AppConfig{},
}

func (m *ConfigManager) Update(newCfg *AppConfig) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.Active = newCfg
}

func (m *ConfigManager) Get() *AppConfig {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.Active
}
```

业务读取：

```go
enabled := config.GlobalManager.Get().
    SystemSwitches.
    EnableRegister
```

指针不是“自动传遍项目”，而是 `GlobalManager` 是包级全局变量，初始化时把它的 `Active` 指向了当前配置。

### 20.5 当前配置注意事项

- `enable_register` 已在注册接口中使用；
- `maintenance_mode` 目前只有加载和变更日志，没有中间件真正拦截请求；
- `jwt.expire` 没有控制 Token TTL，实际 TTL 是 `token.go` 中的常量；
- 配置文件含敏感配置，生产环境应使用环境变量、密钥管理或项目已有的 `ENC(...)` 机制，并及时更换已经暴露的密钥。

---

## 21. 缓存、队列、熔断器等预留能力

### 21.1 多级缓存

项目初始化了：

```text
L1 BigCache → L2 Redis → Singleflight → 数据库回源
```

调用方式：

```go
value, err := cache.MC.GetOrLoad(
    ctx,
    "user:15",
    10*time.Minute,
    func() (string, error) {
        user, err := userRepo.FindByID(15)
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return "", cache.ErrNotFound
        }
        if err != nil {
            return "", err
        }
        bytes, err := json.Marshal(user)
        return string(bytes), err
    },
)
```

更新数据库后删除缓存：

```go
cache.MC.Delete(ctx, "user:15")
```

当前用户、角色、菜单业务没有真正调用 `cache.MC.GetOrLoad`，所以缓存属于已初始化但未接入业务。

### 21.2 Redis Stream 队列

发布任务：

```go
producer := queue.NewProducer(token.RDB)
messageID, err := producer.Publish(ctx, queue.Task{
    Stream: "stream:mail",
    Payload: map[string]any{
        "user_id": user.ID,
        "email":   user.Email,
    },
})
```

消费任务：

```go
consumer := queue.NewConsumer(
    token.RDB,
    "stream:mail",
    "mail_workers",
)

consumer.StartConsume(func(payload map[string]any) error {
    // 返回 nil：处理成功并 ACK
    // 返回 error：重新入队，超过 3 次进入死信队列
    return sendMail(payload)
})
```

当前 main 只启动延时调度和队列指标，没有启动具体 SMS、Mail、File 消费者，也没有业务生产者投递任务。

### 21.3 熔断器

```go
result, err := circuitbreaker.Execute(
    circuitbreaker.ExternalAPIBreaker,
    func() (string, error) {
        return callExternalAPI()
    },
    "fallback value",
)
```

当前 GitHub、短信等业务没有使用熔断器。

### 21.4 其他尚未接入的代码

- `OperationLog` 表会迁移，但没有业务写入操作日志；
- Coupon 有 Model/Repository/Service/Handler 脚手架，但没有 Wire 注入、路由和迁移；
- `permissions.yaml` 没有加载；
- 当前没有 `*_test.go`，`go test ./...` 只能验证所有包可以编译。

---

## 22. 前端 Token 自动刷新

前端 Axios 请求拦截器自动添加 Access Token：

```ts
request.interceptors.request.use((config) => {
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})
```

后端返回业务码 20000 或 20001 时刷新：

```ts
if (res.code === 20000 || res.code === 20001) {
  const refreshToken = getRefreshToken()

  if (refreshToken && !isRefreshing) {
    return refreshAccessToken().then((newToken) => {
      response.config.headers.Authorization = `Bearer ${newToken}`
      // 用新 Access Token 重新发送原来的请求
      return request(response.config)
    })
  }
}
```

并发请求只让第一个请求刷新，其他请求排队：

```ts
if (isRefreshing) {
  return new Promise((resolve) => {
    addRefreshSubscriber((newToken: string) => {
      response.config.headers.Authorization = `Bearer ${newToken}`
      resolve(request(response.config))
    })
  })
}
```

管理后台和普通用户端使用不同的 localStorage key，所以两个前端可以分别保存登录状态。

路由守卫只是检查 Token 字符串是否存在，并不会本地验证 JWT 是否过期。真正过期后由后端返回 20001，再触发 Axios 自动刷新。

---

## 23. 当前项目需要注意的问题及推荐写法

这一节是复制代码前必须看的部分。

### 23.1 查询“不存在”不能等同于所有错误

不推荐：

```go
if _, err := repo.FindByUsername(username); err != nil {
    // 这里把数据库断开也误认为用户名不存在
    // 然后继续创建用户
}
```

推荐：

```go
_, err := repo.FindByUsername(username)
switch {
case err == nil:
    return errors.New("用户名已存在")
case errors.Is(err, gorm.ErrRecordNotFound):
    // 正常情况：用户名可用，继续执行
default:
    return fmt.Errorf("查询用户名失败: %w", err)
}
```

### 23.2 多表操作要使用事务

注册同时涉及用户、认证渠道、角色关系。任何一步失败都应回滚：

```go
func (s *AuthService) RegisterWithTransaction(
    req RegisterReq,
) (*model.User, error) {
    var createdUser *model.User

    err := s.db.Transaction(func(tx *gorm.DB) error {
        userRepo := repository.NewUserRepository(tx)
        roleRepo := repository.NewRoleRepository(tx)
        authRepo := repository.NewUserAuthRepository(tx)

        // 1. 正确判断用户名查询结果
        _, err := userRepo.FindByUsername(req.Username)
        if err == nil {
            return errors.New("用户名已存在")
        }
        if !errors.Is(err, gorm.ErrRecordNotFound) {
            return err
        }

        // 2. 创建用户
        hashedPassword, err := utils.HashPassword(req.Password)
        if err != nil {
            return err
        }
        user := &model.User{
            Username: req.Username,
            Password: hashedPassword,
            Nickname: req.Nickname,
            Email:    req.Email,
            Phone:    req.Phone,
            Status:   1,
        }
        if err := userRepo.Create(user); err != nil {
            return err
        }

        // 3. 创建 password 登录渠道
        auth := &model.UserAuth{
            UserID:       user.ID,
            IdentityType: "password",
            Identifier:   user.Username,
            Credential:   user.Password,
        }
        if err := authRepo.Create(auth); err != nil {
            return err
        }

        // 4. 分配默认 user 角色
        userRole, err := roleRepo.FindByCode("user")
        if err != nil {
            return err
        }
        if err := userRepo.AssignRoles(
            user.ID,
            []uint64{userRole.ID},
        ); err != nil {
            return err
        }

        createdUser = user
        return nil
    })
    if err != nil {
        return nil, err
    }

    // Casbin 不在同一个 GORM 事务中，数据库提交成功后再同步，
    // 同步失败要记录日志并安排补偿/重试。
    userIDString := strconv.FormatUint(createdUser.ID, 10)
    if _, err := casbin.AddRoleForUser(userIDString, "user"); err != nil {
        return nil, fmt.Errorf("用户已创建，但同步权限失败: %w", err)
    }

    return createdUser, nil
}
```

### 23.3 Handler 不要直接绑定数据库 Model

不推荐：

```go
var user model.User
c.ShouldBindJSON(&user)
```

前端可能传入本不应允许修改的 `id`、`is_admin`、`status` 等字段。

推荐：

```go
type CreateUserReq struct {
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required,min=6"`
    Nickname string `json:"nickname"`
    Email    string `json:"email" binding:"omitempty,email"`
}
```

DTO 只暴露这个接口真正允许前端传入的字段。

### 23.4 ID 解析错误不能忽略

不推荐：

```go
id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
```

解析失败时 `id` 会变成 0，后面可能返回误导性的“数据不存在”。

推荐：

```go
id, err := strconv.ParseUint(c.Param("id"), 10, 64)
if err != nil || id == 0 {
    response.Fail(c, response.ErrParamInvalid, "ID 格式错误")
    return
}
```

### 23.5 不要忽略返回的错误

不推荐：

```go
roles, _ := repo.GetUserRoles(userID)
pair, _ := token.GenerateTokenPair(userID, username, roles)
token.BlacklistToken(jti, ttl)
```

推荐：

```go
roles, err := repo.GetUserRoles(userID)
if err != nil {
    return nil, fmt.Errorf("查询角色失败: %w", err)
}

pair, err := token.GenerateTokenPair(userID, username, roleNames)
if err != nil {
    return nil, fmt.Errorf("生成 Token 失败: %w", err)
}
```

### 23.6 `map[string]interface{}` 类型断言可能 panic

当前角色更新代码类似：

```go
role.Name = updates["name"].(string)
```

如果前端传 `{"name": null}` 或数字，会 panic。

推荐 DTO：

```go
type UpdateRoleReq struct {
    Name *string `json:"name" binding:"omitempty,max=50"`
    Desc *string `json:"desc" binding:"omitempty,max=100"`
}
```

### 23.7 菜单为空不能回退为全部菜单

推荐：

```go
if len(menus) == 0 {
    return []*utils.MenuTreeResp{}, nil
}
```

### 23.8 权限修改要区分 `g` 和 `p`

```text
DeleteRolesForUser(userID)       删除 g：用户拥有哪些角色
DeletePermissionsForUser(role)   删除 p：角色拥有哪些权限
```

### 23.9 用户、角色、权限状态要真正参与鉴权

仅修改数据库的 `status` 不代表权限立即失效。需要在业务上选择：

- 禁用时删除/停用 Casbin 规则；或
- JWTAuth 每次查询用户和角色状态；或
- 给用户权限状态建立短 TTL 缓存并主动失效。

### 23.10 删除业务先考虑关联数据

删除前至少检查：

```text
是否允许删除系统内置数据
是否有子节点
是否有多对多关联
是否有 Casbin 规则
是否有文件
是否应该软删除
是否需要事务
```

### 23.11 Token 安全

- 退出时同时处理 Access Token 和 Refresh Token；
- 刷新后使旧 Refresh Token 失效，实现轮换；
- 修改密码、禁用账户时撤销旧 Token；
- Redis 不可用时要明确选择“安全失败”还是“降级可用”；
- 不在日志中输出完整 Token。

---

## 24. 新增一个业务接口的完整模板

下面用“公告 Notice”作为示例。它不是当前已注册功能，而是一份可以参考的完整开发顺序。

### 24.1 Model

```go
// backend/internal/model/notice.go
type Notice struct {
    BaseModel
    Title   string `gorm:"size:100;not null" json:"title"`
    Content string `gorm:"type:text;not null" json:"content"`
    Status  int8   `gorm:"default:1" json:"status"`
}

func (Notice) TableName() string {
    return "sys_notice"
}
```

并加入 AutoMigrate：

```go
db.AutoMigrate(
    // ...已有 Model
    &model.Notice{},
)
```

### 24.2 请求 DTO

```go
type CreateNoticeReq struct {
    Title   string `json:"title" binding:"required,max=100"`
    Content string `json:"content" binding:"required"`
}

type NoticeListReq struct {
    Keyword  string `form:"keyword"`
    Page     int    `form:"page"`
    PageSize int    `form:"page_size"`
}
```

### 24.3 Repository

```go
type NoticeRepository struct {
    db *gorm.DB
}

func NewNoticeRepository(db *gorm.DB) *NoticeRepository {
    return &NoticeRepository{db: db}
}

func (r *NoticeRepository) Create(notice *model.Notice) error {
    return r.db.Create(notice).Error
}

func (r *NoticeRepository) List(
    keyword string,
    page int,
    pageSize int,
) ([]model.Notice, int64, error) {
    var list []model.Notice
    var total int64

    query := r.db.Model(&model.Notice{})
    if keyword != "" {
        query = query.Where("title LIKE ?", "%"+keyword+"%")
    }

    if err := query.Count(&total).Error; err != nil {
        return nil, 0, err
    }

    offset := (page - 1) * pageSize
    err := query.Order("id DESC").
        Offset(offset).
        Limit(pageSize).
        Find(&list).Error

    return list, total, err
}
```

### 24.4 Service

```go
type NoticeService struct {
    noticeRepo *repository.NoticeRepository
}

func NewNoticeService(
    noticeRepo *repository.NoticeRepository,
) *NoticeService {
    return &NoticeService{noticeRepo: noticeRepo}
}

func (s *NoticeService) Create(req CreateNoticeReq) (*model.Notice, error) {
    notice := &model.Notice{
        Title:   strings.TrimSpace(req.Title),
        Content: req.Content,
        Status:  1,
    }

    if notice.Title == "" {
        return nil, errors.New("公告标题不能为空")
    }

    if err := s.noticeRepo.Create(notice); err != nil {
        return nil, fmt.Errorf("创建公告失败: %w", err)
    }

    return notice, nil
}

func (s *NoticeService) List(
    req NoticeListReq,
) ([]model.Notice, int64, error) {
    if req.Page <= 0 {
        req.Page = 1
    }
    if req.PageSize <= 0 {
        req.PageSize = 10
    }
    if req.PageSize > 100 {
        req.PageSize = 100
    }

    return s.noticeRepo.List(req.Keyword, req.Page, req.PageSize)
}
```

### 24.5 Handler

```go
type NoticeHandler struct {
    noticeSvc *service.NoticeService
}

func NewNoticeHandler(
    noticeSvc *service.NoticeService,
) *NoticeHandler {
    return &NoticeHandler{noticeSvc: noticeSvc}
}

func (h *NoticeHandler) Create(c *gin.Context) {
    var req service.CreateNoticeReq
    if !response.ShouldBindJSON(c, &req) {
        return
    }

    notice, err := h.noticeSvc.Create(req)
    if err != nil {
        response.Fail(c, response.ErrSystemError, err.Error())
        return
    }

    zap.L().Info("创建公告成功",
        zap.Uint64("notice_id", notice.ID),
        zap.String("trace_id", c.GetString("trace_id")),
    )

    response.Success(c, notice)
}

func (h *NoticeHandler) List(c *gin.Context) {
    var req service.NoticeListReq
    if err := c.ShouldBindQuery(&req); err != nil {
        response.Fail(c, response.ErrParamInvalid, "查询参数错误")
        return
    }

    list, total, err := h.noticeSvc.List(req)
    if err != nil {
        response.Fail(c, response.ErrSystemError, err.Error())
        return
    }

    response.Success(c, gin.H{
        "list":  list,
        "total": total,
    })
}
```

### 24.6 Wire 注入

```go
var RepositorySet = wire.NewSet(
    // ...已有 Repository
    repository.NewNoticeRepository,
)

var ServiceSet = wire.NewSet(
    // ...已有 Service
    service.NewNoticeService,
)

var HandlerSet = wire.NewSet(
    // ...已有 Handler
    handler.NewNoticeHandler,
)
```

修改 `wire.go` 后执行：

```bash
cd backend
wire ./cmd/server
```

生成新的 `wire_gen.go`。

### 24.7 注册路由

```go
func NewRouter(
    // ...已有参数
    noticeHandler *handler.NoticeHandler,
) *gin.Engine {
    // ...

    notices := v1.Group("/notices")
    {
        notices.GET("", noticeHandler.List)
        notices.POST("", noticeHandler.Create)
    }

    return r
}
```

### 24.8 增加权限

权限定义：

```text
GET  /api/v1/notices  查询公告
POST /api/v1/notices  创建公告
```

Casbin 规则示例：

```text
p, admin, /api/v1/notices, GET
p, admin, /api/v1/notices, POST
```

如果普通用户可以查看公告：

```go
casbin.AddPolicy("user", "/api/v1/notices", "GET")
```

### 24.9 最少测试场景

```text
创建成功
标题为空
标题过长
未携带 Token
Token 过期
有角色但无 POST 权限
有 POST 权限
数据库写入失败
分页默认值
page_size 超过上限
```

---

## 25. 开发新需求时的检查清单

### 25.1 先明确业务

```text
谁可以调用？
公开接口还是登录接口？
需要什么角色和权限？
参数来自 JSON、Query、Path、Header 还是文件？
成功后要写哪些表？
是否有唯一性、状态、数量限制？
是否需要事务？
失败时前端应该看到什么？
是否要写日志、缓存、消息队列？
是否存在重复请求，需要幂等吗？
```

### 25.2 编码顺序

```text
1. Model / 数据表
2. 请求 DTO 和返回 DTO
3. Repository 数据库操作
4. Service 业务规则
5. Handler 参数和响应
6. Wire 依赖注入
7. Router 注册
8. Permission 定义
9. Casbin 角色授权
10. 菜单和前端调用
11. 日志、事务、缓存、队列
12. 测试正常和异常分支
```

### 25.3 每个 Service 方法至少考虑

```text
参数是否合法
数据是否存在
数据是否重复
对象是否启用
当前用户是否有权操作目标对象
数据库错误和“记录不存在”是否区分
多步写操作失败是否回滚
外部服务失败是否重试、熔断或降级
成功后是否要清缓存
敏感数据是否会进入日志或响应
```

### 25.4 每个 Handler 至少考虑

```text
是否检查 Bind 错误
是否检查 Path ID 转换错误
调用 Service 出错后是否立即 return
错误码是否符合业务含义
响应是否泄露密码、凭证、Token
是否能从 current_user_id 获取当前用户
是否记录了 trace_id 和关键业务 ID
```

### 25.5 最重要的一句话

```text
Handler 负责 HTTP，Service 负责业务，Repository 负责数据库，
Middleware 负责通用拦截，Casbin 负责最终 API 权限。
```
