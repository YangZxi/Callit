import { Link as RouterLink, NavLink, useLocation } from "react-router-dom";
import { Link as HeroLink } from "@heroui/react";
import clsx from "clsx";

// import Login from "./login";
import { GithubIcon, Logo } from "@/components/icons";
import { ThemeSwitch } from "@/components/theme-switch";
import { siteConfig } from "@/config/site";

function AppNavbar() {
  const location = useLocation();

  return (
    <nav
      className={clsx(
        "bg-transparent absolute",
        "md:min-w-120 top-[10px] md:top-[20px]",
        "z-50 w-auto mx-auto inset-x-0",
        "bg-background/70 backdrop-blur-xl backdrop-saturate-150",
        "rounded-full border border-default-200/50 shadow-lg",
        "max-w-fit px-4",
      )}
    >
      <div className="px-0 gap-6 h-12 flex items-center justify-between">
        <div className="basis-1/5 sm:basis-full flex items-center justify-start gap-3 max-w-fit mr-2">
          <RouterLink className="flex justify-start items-center gap-1 no-underline" to={`${window.__BASE_PREFIX__}/`}>
            <Logo />
            <p className="font-bold text-inherit hidden sm:block">{siteConfig.name}</p>
          </RouterLink>
        </div>
        <ul className="flex gap-6 justify-start ml-2">
          {siteConfig.navItems.map((item) => {
            const active = location.pathname === item.href;
            return (
              <li key={item.href}>
                <NavLink
                  className={clsx(
                    `${item.mobile === true ? "" : "hidden sm:inline-block"}`,
                    "no-underline opacity-80 hover:opacity-100 transition-opacity",
                    active ? "text-primary font-medium opacity-100" : "text-default-700",
                  )}
                  to={item.href}
                >
                  {item.label}
                </NavLink>
              </li>
            );
          })}
        </ul>
      
        <div className="sm:flex basis-1/5 sm:basis-full items-center justify-end">
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
