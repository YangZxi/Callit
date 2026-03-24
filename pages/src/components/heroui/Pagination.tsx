import clsx from "clsx";
import { Pagination as HeroPagination } from "@heroui/react";

type PaginationProps = {
  className?: string;
  ariaLabel?: string;
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  isDisabled?: boolean;
};

export default function Pagination({
  className,
  ariaLabel = "Pagination",
  page,
  totalPages,
  onPageChange,
  isDisabled = false,
}: PaginationProps) {
  const getPageNumbers = () => {
    if (totalPages <= 7) {
      return Array.from({ length: totalPages }, (_, index) => index + 1);
    }

    const pages: Array<number | "ellipsis"> = [];

    pages.push(1);

    if (page > 3) {
      pages.push("ellipsis");
    }

    const start = Math.max(2, page - 1);
    const end = Math.min(totalPages - 1, page + 1);

    for (let currentPage = start; currentPage <= end; currentPage += 1) {
      pages.push(currentPage);
    }

    if (page < totalPages - 2) {
      pages.push("ellipsis");
    }

    pages.push(totalPages);

    return pages;
  };

  return (
    <HeroPagination aria-label={ariaLabel} className={clsx("justify-center", className)}>
      <HeroPagination.Content>
        <HeroPagination.Item>
          <HeroPagination.Previous
            isDisabled={isDisabled || page <= 1}
            onPress={() => onPageChange(page - 1)}
          >
            <HeroPagination.PreviousIcon />
            {/* <span>{previousLabel}</span> */}
          </HeroPagination.Previous>
        </HeroPagination.Item>
        {getPageNumbers().map((item, index) => (
          item === "ellipsis" ? (
            <HeroPagination.Item key={`ellipsis-${index}`}>
              <HeroPagination.Ellipsis />
            </HeroPagination.Item>
          ) : (
            <HeroPagination.Item key={item}>
              <HeroPagination.Link
                isActive={item === page}
                onPress={() => onPageChange(item)}
              >
                {item}
              </HeroPagination.Link>
            </HeroPagination.Item>
          )
        ))}
        <HeroPagination.Item>
          <HeroPagination.Next
            isDisabled={isDisabled || page >= totalPages}
            onPress={() => onPageChange(page + 1)}
          >
            {/* <span>{nextLabel}</span> */}
            <HeroPagination.NextIcon />
          </HeroPagination.Next>
        </HeroPagination.Item>
      </HeroPagination.Content>
    </HeroPagination>
  );
}
