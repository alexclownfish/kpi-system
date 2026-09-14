package handlers

import (
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"dootask-kpi-server/models"
	"dootask-kpi-server/utils"
)

// JWT密钥 - 生产环境中应该使用环境变量
var jwtSecret = []byte("your-secret-key-change-in-production")

// JWT Claims结构
type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// 登录请求结构
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// 用户登录（DooTaskToken）请求结构
type LoginByDooTaskTokenRequest struct {
	Email string `json:"email" binding:"required,email"`
	Token string `json:"token" binding:"required"`
}

// 注册请求结构
type RegisterRequest struct {
	Name         string `json:"name" binding:"required"`
	Email        string `json:"email" binding:"required,email"`
	Password     string `json:"password" binding:"required,min=6"`
	Position     string `json:"position"`
	DepartmentID uint   `json:"department_id" binding:"required"`
}

// 登录响应结构
type LoginResponse struct {
	Token string           `json:"token"`
	User  *models.Employee `json:"user"`
}

// 生成JWT token
func generateToken(user *models.Employee) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour) // 24小时过期
	claims := &Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "dootask-kpi",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// 验证JWT token
func verifyToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, jwt.ErrSignatureInvalid
	}

	return claims, nil
}

// Register 用户注册
func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查是否允许注册
	var allowRegistrationSetting models.SystemSetting
	if err := models.DB.Where("key = ?", "allow_registration").First(&allowRegistrationSetting).Error; err == nil {
		if allowRegistrationSetting.Value != "true" {
			c.JSON(http.StatusForbidden, gin.H{"error": "系统当前未开放注册"})
			return
		}
	}

	// 检查邮箱是否已存在
	var existingUser models.Employee
	if err := models.DB.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "邮箱已被注册"})
		return
	}

	// 检查部门是否存在
	var department models.Department
	if err := models.DB.First(&department, req.DepartmentID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "部门不存在"})
		return
	}

	// 查找上级
	var manager models.Employee
	if err := models.DB.Where("department_id = ? AND role in ?", req.DepartmentID, []string{"manager", "hr"}).First(&manager).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "部门不存在上级"})
		return
	}

	// 密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	// 创建用户
	user := models.Employee{
		Name:         req.Name,
		Email:        req.Email,
		Password:     string(hashedPassword),
		Position:     req.Position,
		DepartmentID: req.DepartmentID,
		ManagerID:    &manager.ID,
		Role:         "employee", // 默认角色为员工
		IsActive:     true,
	}

	if err := models.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "用户创建失败"})
		return
	}

	// 预加载关联数据
	if err := models.DB.Preload("Department").First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "用户数据加载失败"})
		return
	}

	// 生成token
	token, err := generateToken(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token生成失败"})
		return
	}

	c.JSON(http.StatusCreated, LoginResponse{
		Token: token,
		User:  &user,
	})
}

// Login 用户登录
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 查找用户
	var user models.Employee
	if err := models.DB.Preload("Department").Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "邮箱或密码错误"})
		return
	}

	// 检查用户是否激活
	if !user.IsActive {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账户已被禁用"})
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "邮箱或密码错误"})
		return
	}

	// 生成token
	token, err := generateToken(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token生成失败"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token: token,
		User:  &user,
	})
}

// LoginByDooTaskToken 用户登录（DooTaskToken）
// 1、如果用户不存在，则创建用户
//   - 创建用户时，如果部门不存在，则创建部门
//   - 如果是管理员，则设置为HR
//   - 如果是员工，且是部门负责人，则设置为经理
//
// 2、如果用户存在，则更新用户信息
//   - 只更新用户名、职位
func LoginByDooTaskToken(c *gin.Context) {
	var req LoginByDooTaskTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 连接 DooTask
	dooTaskClient := utils.NewDooTaskClient(req.Token)
	dooTaskUser, err := dooTaskClient.Client.GetUserInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "验证身份失败"})
		return
	}

	// 查找用户
	var user models.Employee
	if err := models.DB.Preload("Department").Where("email = ?", req.Email).First(&user).Error; err != nil {
		// 如果用户不存在
		user = models.Employee{
			Name:          dooTaskUser.Nickname,
			Email:         req.Email,
			DooTaskUserID: &dooTaskUser.UserID,
			Position:      dooTaskUser.Profession,
		}
		if slices.Contains(dooTaskUser.Identity, "admin") {
			user.Role = "hr"
		} else {
			user.Role = "employee"
		}

		// 检测部门
		if departments, err := dooTaskClient.Client.GetUserDepartments(); err == nil {
			if len(departments) > 0 {
				var existingDepartment models.Department
				if err := models.DB.Where("name = ?", departments[0].Name).First(&existingDepartment).Error; err != nil {
					existingDepartment = models.Department{
						Name: departments[0].Name,
					}
					if err := models.DB.Create(&existingDepartment).Error; err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "部门创建失败"})
						return
					}
				}

				// 设置部门ID
				user.DepartmentID = existingDepartment.ID

				// 如果用户是员工，且是部门负责人，则设置为经理
				if user.Role == "employee" && departments[0].OwnerUserID == dooTaskUser.UserID {
					user.Role = "manager"
				}
			}
		}

		// 创建用户
		if err := models.DB.Create(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "用户创建失败"})
			return
		}
	} else {
		// 更新用户信息
		user.Name = dooTaskUser.Nickname
		user.DooTaskUserID = &dooTaskUser.UserID
		user.Position = dooTaskUser.Profession
		models.DB.Save(&user)
	}

	// 检查用户是否激活
	if !user.IsActive {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "账户已被禁用"})
		return
	}

	// 生成token
	token, err := generateToken(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token生成失败"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token: token,
		User:  &user,
	})
}

// GetCurrentUser 获取当前用户信息
func GetCurrentUser(c *gin.Context) {
	// 从中间件获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户未登录"})
		return
	}

	var user models.Employee
	if err := models.DB.Preload("Department").First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// RefreshToken 刷新token
func RefreshToken(c *gin.Context) {
	tokenString := c.GetHeader("Authorization")
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少认证token"})
		return
	}

	// 移除Bearer前缀
	if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
		tokenString = tokenString[7:]
	}

	// 验证token（即使过期也要能解析）
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil && !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的token"})
		return
	}

	// 检查用户是否仍然存在
	var user models.Employee
	if err := models.DB.First(&user, claims.UserID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
		return
	}

	// 生成新的token
	newToken, err := generateToken(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token生成失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": newToken,
	})
}

// AuthMiddleware JWT认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 标记为已认证
		if c.GetBool("is_authenticated") {
			return
		}
		c.Set("is_authenticated", true)

		// 从请求头获取token
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少认证token"})
			c.Abort()
			return
		}

		// 移除Bearer前缀
		if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
			tokenString = tokenString[7:]
		}

		// 验证token
		claims, err := verifyToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的token"})
			c.Abort()
			return
		}

		// 检查用户是否仍然存在且激活
		var user models.Employee
		if err := models.DB.First(&user, claims.UserID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			c.Abort()
			return
		}

		if !user.IsActive {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "账户已被禁用"})
			c.Abort()
			return
		}

		// 将用户信息存储在context中
		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)
		c.Set("user_name", user.Name)
		c.Next()
	}
}

// RoleMiddleware 角色权限中间件
func RoleMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 先执行认证中间件
		AuthMiddleware()(c)

		userRole, exists := c.Get("user_role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户未登录"})
			c.Abort()
			return
		}

		role := userRole.(string)
		for _, allowedRole := range allowedRoles {
			if role == allowedRole {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "权限不足"})
		c.Abort()
	}
}
