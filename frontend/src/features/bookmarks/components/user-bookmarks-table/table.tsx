import Link from "next/link";
import {
  Table,
  TableTr,
  TableTd,
  TableTh,
  TableThead,
  TableTbody,
  ActionIcon,
  ActionIconGroup,
  Tooltip,
  Group,
  rem,
} from "@mantine/core";
import {PermissionGuard} from "@/components/permission-guard";
import {Pagination} from "@/components/pagination";
import {MyBookmarkDeleteButton} from "./delete-button";
import {IconEye} from "@tabler/icons-react";
import {fetchUserBookmarks} from "@/dal/private/bookmarks";
import {dateFromNow} from "@/lib/date-and-time";
import {APP_PATHS} from "@/lib/app-paths";

export const TABLE_HEADERS = ["#", "عنوان", "تاریخ ثبت", "عملیات"];

type Props = {
  page: number | string;
};

export async function UserBookmarksTable({page}: Props) {
  const bookmarksResponse = await fetchUserBookmarks({
    params: {
      page: page,
    },
  });
  const bookmarks = bookmarksResponse.items;
  const {total_pages, current_page} = bookmarksResponse.pagination;

  return (
    <>
      <Table verticalSpacing={"sm"} striped withRowBorders>
        <TableThead>
          <TableTr>
            {TABLE_HEADERS.map((h) => {
              return <TableTh key={h}>{h}</TableTh>;
            })}
          </TableTr>
        </TableThead>
        <TableTbody>
          {bookmarks.length === 0 && (
            <TableTr>
              <TableTd colSpan={TABLE_HEADERS.length} ta={"center"}>
                هنوز چیزی را ذخیره نکرده اید
              </TableTd>
            </TableTr>
          )}
          {bookmarks.map((bookmark: any, index: number) => {
            return (
              <TableTr key={bookmark.object_uuid}>
                <TableTd>{index + 1}</TableTd>
                <TableTd>{bookmark.title}</TableTd>
                <TableTd>{dateFromNow(bookmark.created_at)}</TableTd>
                <TableTd>
                  <ActionIconGroup>
                    <Tooltip label={"بازدید کردن کامنت"} withArrow>
                      <ActionIcon
                        variant="light"
                        size="lg"
                        color="blue"
                        aria-label="بازدید کردن کامنت"
                        component={Link}
                        href={`${APP_PATHS.articles.detail(bookmark.object_uuid)}`}
                      >
                        <IconEye style={{width: rem(20)}} stroke={1.5} />
                      </ActionIcon>
                    </Tooltip>
                    <PermissionGuard
                      allowedPermissions={["self.bookmarks.delete"]}
                    >
                      <MyBookmarkDeleteButton
                        title={bookmark.title}
                        bookmarkID={bookmark.object_uuid}
                      />
                    </PermissionGuard>
                  </ActionIconGroup>
                </TableTd>
              </TableTr>
            );
          })}
        </TableTbody>
      </Table>
      {bookmarks.length >= 1 && (
        <Group mt="md" mb={"lg"} justify="flex-end">
          <Pagination total={total_pages} current={current_page} />
        </Group>
      )}
    </>
  );
}
