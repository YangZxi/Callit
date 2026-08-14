import { Link as RouterLink, NavLink, useLocation, useNavigate } from "react-router-dom";
import { Dropdown, Link as HeroLink } from "@heroui/react";
import clsx from "clsx";
import { useTranslation } from "react-i18next";

// import Login from "./login";
import { GithubIcon } from "@/components/icons";
import { ThemeSwitch } from "@/components/theme-switch";
import { LanguageSwitch } from "@/components/language-switch";
import { siteConfig } from "@/config/site";

function AppNavbar() {
  const location = useLocation();
  const navigate = useNavigate();
  const { t } = useTranslation();

  return (
    <nav
      className={clsx(
        "bg-transparent absolute",
        "md:min-w-120 top-[10px] md:top-[20px]",
        "z-50 mx-auto inset-x-0 w-fit max-w-[calc(100%-1rem)]",
        "bg-background/70 backdrop-blur-xl backdrop-saturate-150",
        "rounded-full border border-default-200/50 shadow-lg",
        "px-3 sm:px-4",
      )}
    >
      <div className="h-12 flex items-center gap-3 sm:gap-5">
        <div className="shrink-0 flex items-center justify-start">
          <RouterLink className="flex justify-start items-center gap-1 no-underline" to={`${window.__BASE_PREFIX__}/`}>
            <img
              src={`${window.__BASE_PREFIX__}/static/logo.png`}
              alt="Logo"
              className="h-6 w-6 rounded-sm object-cover"
            />
            <p className="font-bold text-inherit hidden sm:block">{siteConfig.name}</p>
          </RouterLink>
        </div>
        <ul className="flex min-w-0 items-center gap-3 sm:gap-5">
          {siteConfig.navItems.map((item) => {
            const children = "children" in item ? item.children : undefined;
            const active = children
              ? children.some((child) => location.pathname === child.href)
              : location.pathname === item.href;
            if (children) {
              return (
                <li key={item.href} className="shrink-0">
                  <Dropdown>
                    <Dropdown.Trigger>
                      <div
                        className={clsx(
                          `${item.mobile === true ? "" : "hidden sm:inline-block"}`,
                          "whitespace-nowrap no-underline opacity-80 hover:opacity-100 transition-opacity",
                          active ? "text-primary font-medium opacity-100" : "text-default-700",
                        )}
                      >
                        {t(item.labelKey)}
                      </div>
                    </Dropdown.Trigger>
                    <Dropdown.Popover 
                      placement="bottom" 
                      className="min-w-[90px] bg-transparent bg-background/70 backdrop-blur-xl backdrop-saturate-150"
                    >
                      <Dropdown.Menu aria-label={t(item.labelKey)}>
                        {children.map((child) => (
                          <Dropdown.Item
                            key={child.href}
                            id={child.href}
                            textValue={t(child.labelKey)}
                            onPress={() => navigate(child.href)}
                          >
                            {t(child.labelKey)}
                          </Dropdown.Item>
                        ))}
                      </Dropdown.Menu>
                    </Dropdown.Popover>
                  </Dropdown>
                </li>
              );
            }
            return (
              <li key={item.href} className="shrink-0">
                <NavLink
                  className={clsx(
                    `${item.mobile === true ? "" : "hidden sm:inline-block"}`,
                    "whitespace-nowrap no-underline opacity-80 hover:opacity-100 transition-opacity",
                    active ? "text-primary font-medium opacity-100" : "text-default-700",
                  )}
                  to={item.href}
                >
                  {t(item.labelKey)}
                </NavLink>
              </li>
            );
          })}
        </ul>

        <div className="ml-auto flex shrink-0 items-center justify-end">
          <div className="h-6 w-[1px] bg-default-300 mx-1 hidden sm:block" />
          <div className="flex gap-2 items-center">
            <HeroLink
              target="_blank"
              aria-label="Github"
              className="hidden md:block text-default-500 hover:text-foreground transition-colors"
              href="https://github.com/yangzxi/callit"
            >
              <GithubIcon />
            </HeroLink>
            <ThemeSwitch />
            <LanguageSwitch />
          </div>

          {/* <div>
          <Login />
        </div> */}
        </div>
      </div>
    </nav>
  );
}

export default AppNavbar;
