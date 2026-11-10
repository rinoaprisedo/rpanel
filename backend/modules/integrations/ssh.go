package integrations

import (
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"rpanel/backend/modules/activity_logs"
	"rpanel/backend/utils"

	"github.com/gin-gonic/gin"
)

// A bare public key: type, base64 blob, optional comment. No options prefix (command="..." would run something else) and no newline.
var pubKeyRe = regexp.MustCompile(`^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(256|384|521)|sk-ssh-ed25519@openssh\.com) [A-Za-z0-9+/=]{20,1000}( [A-Za-z0-9 ._@:+=-]{0,100})?$`)

func (h *Deploy) allowedUser(name string) bool {
	for _, u := range h.sshOK {
		if u == name {
			return true
		}
	}
	return false
}

func (h *Deploy) SSHInfo(c *gin.Context) {
	utils.Success(c, 200, "ok", gin.H{"users": h.sshOK})
}

type sshInput struct {
	User      string `json:"user" binding:"required"`
	PublicKey string `json:"public_key" binding:"required"`
}

func (h *Deploy) sshTarget(c *gin.Context) (in sshInput, u *user.User, key string, ok bool) {
	if c.ShouldBindJSON(&in) != nil {
		utils.Error(c, 400, "invalid request")
		return
	}
	key = strings.TrimSpace(in.PublicKey)
	if !pubKeyRe.MatchString(key) {
		utils.Error(c, 400, "not a valid SSH public key")
		return
	}
	if !h.allowedUser(in.User) {
		utils.Error(c, 403, "this user is not allowed for key registration (see DEPLOY_SSH_USERS)")
		return
	}
	u, err := user.Lookup(in.User)
	if err != nil {
		utils.Error(c, 404, "no such system user")
		return
	}
	ok = true
	return
}

// SSHAuthorize adds the public key to the user's authorized_keys (idempotent) so Vibecoder can open a normal ssh session.
func (h *Deploy) SSHAuthorize(c *gin.Context) {
	in, u, key, ok := h.sshTarget(c)
	if !ok {
		return
	}
	dir := filepath.Join(u.HomeDir, ".ssh")
	file := filepath.Join(dir, "authorized_keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		utils.Error(c, 422, "could not prepare ~/.ssh")
		return
	}
	cur, _ := os.ReadFile(file)
	blob := strings.Fields(key)[1]
	already := false
	for _, l := range strings.Split(string(cur), "\n") {
		if f := strings.Fields(l); len(f) >= 2 && f[1] == blob {
			already = true
		}
	}
	if !already {
		text := string(cur)
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		if err := os.WriteFile(file, []byte(text+key+"\n"), 0o600); err != nil {
			utils.Error(c, 422, "could not write authorized_keys")
			return
		}
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	_ = os.Chown(dir, uid, gid)
	_ = os.Chown(file, uid, gid)
	audit(c, activity_logs.ActionCreate, in.User, gin.H{"op": "ssh authorize", "fingerprint_blob": blob[:16], "already": already})
	utils.Success(c, 200, "ok", gin.H{"user": in.User, "added": !already})
}

// SSHRevoke removes the key again.
func (h *Deploy) SSHRevoke(c *gin.Context) {
	in, u, key, ok := h.sshTarget(c)
	if !ok {
		return
	}
	file := filepath.Join(u.HomeDir, ".ssh", "authorized_keys")
	cur, err := os.ReadFile(file)
	if err != nil {
		utils.Success(c, 200, "ok", gin.H{"removed": false})
		return
	}
	blob := strings.Fields(key)[1]
	kept, removed := []string{}, false
	for _, l := range strings.Split(strings.TrimRight(string(cur), "\n"), "\n") {
		if f := strings.Fields(l); len(f) >= 2 && f[1] == blob {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	if removed {
		if err := os.WriteFile(file, []byte(strings.Join(kept, "\n")+"\n"), 0o600); err != nil {
			utils.Error(c, 422, "could not write authorized_keys")
			return
		}
		audit(c, activity_logs.ActionDelete, in.User, gin.H{"op": "ssh revoke"})
	}
	utils.Success(c, 200, "ok", gin.H{"removed": removed})
}
