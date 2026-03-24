import { useEffect, useState } from "react";
import { Modal, useDisclosure, toast } from "@heroui/react";
import { Button } from "@heroui/react";
import { Input } from "@/components/heroui";

import api, { ApiResponse } from "@/lib/api";
import { UserProfile } from "@/types/user";

export default function Login() {
  const [user, setUser] = useState<UserProfile | null>(null);
  const { isOpen, onOpen, onOpenChange, onClose } = useDisclosure();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const btnClassName = "text-default-600 justify-start data-[hover=true]:text-black";

  useEffect(() => {
    const fetchUser = async () => {
      try {
        const data = await api.get<UserProfile>("/me", {
          hideToast: true,
        });
        setUser(data);
      } catch {
        setUser(null);
      }
    };
    fetchUser();
  }, []);

  const loginHandler = async () => {
    if (!username.trim() || !password.trim()) return;
    try {
      const profile = await api.post<UserProfile>("/login", {
        username: username.trim(),
        password: password.trim(),
      });
      setUser(profile);
      onClose();
      toast.success("登录成功");
    } catch (err) {
      toast.danger("登录失败", { description: (err as ApiResponse<unknown>).msg });
    }
  };

  const logoutHandler = async () => {
    try {
      await api.post("/logout");
      setUser(null);
      window.location.reload();
    } catch (err) {
    }
  };

  return (
    <>
      {user ? (
        <Button className={btnClassName} variant="tertiary" onPress={logoutHandler}>
          {user.nickname || user.username} 退出登录
        </Button>
      ) : (
        <Button size="sm" variant="ghost" onPress={onOpen}>
          登录
        </Button>
      )}

      <Modal isOpen={isOpen} onOpenChange={onOpenChange}>
        <Modal.Backdrop isDismissable={false} isKeyboardDismissDisabled>
          <Modal.Container>
            <Modal.Dialog>
              <Modal.Header className="flex flex-col gap-1">
                <Modal.Heading>登录</Modal.Heading>
              </Modal.Header>
              <Modal.Body>
                <div className="space-y-3">
                  <Input
                    isRequired
                    label="用户名"
                    name="username"
                    placeholder="请输入用户名户名"
                    type="text"
                    value={username}
                    onValueChange={setUsername}
                  />
                  <Input
                    isRequired
                    label="密码"
                    name="password"
                    placeholder="请输入密码"
                    type="password"
                    value={password}
                    onValueChange={setPassword}
                  />
                </div>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="danger-soft" onPress={onClose}>
                  取消
                </Button>
                <Button
                  variant="primary"
                  onPress={loginHandler}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      loginHandler();
                    }
                  }}
                >
                  登录
                </Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>
    </>
  );
}
