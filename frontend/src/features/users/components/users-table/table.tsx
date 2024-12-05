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
  Stack,
  Button,
  rem,
} from "@mantine/core";
import {UserAvatar} from "@/components/user-avatar";
import {Pagination} from "@/components/pagination";
import {PermissionGuard} from "@/components/permission-guard";
import {DeleteButton} from "./delete-button";
import {IconPencil, IconUserPlus} from "@tabler/icons-react";
import {fetchUsers} from "@/dal/private/users";
import {APP_PATHS} from "@/lib/app-paths";

export const TABLE_HEADERS = [
  "#",
  "آواتار",
  "نام",
  "نام کاربری",
  "ایمیل",
  "عملیات",
];

type Props = {
  page: number | string;
};

export async function UsersTable({page}: Props) {
  const {items: users, pagination} = await fetchUsers({
    params: {
      page: page,
    },
  });
  const {total_pages, current_page} = pagination;

  return (
    <Stack>
      <Group justify="flex-end">
        <Button
          variant="light"
          component={Link}
          leftSection={<IconUserPlus />}
          href={APP_PATHS.dashboard.users.new}
        >
          کاربر جدید
        </Button>
      </Group>
      <Table verticalSpacing={"sm"} striped withRowBorders>
        <TableThead>
          <TableTr>
            {TABLE_HEADERS.map((h) => {
              return <TableTh key={h}>{h}</TableTh>;
            })}
          </TableTr>
        </TableThead>
        <TableTbody>
          {users.length === 0 && (
            <TableTr>
              <TableTd colSpan={TABLE_HEADERS.length} ta={"center"}>
                کاربری وجود ندارد
              </TableTd>
            </TableTr>
          )}
          {users.map((user: any, index: number) => {
            return (
              <TableTr key={user.uuid}>
                <TableTd>{index + 1}</TableTd>
                <TableTd>
                  <UserAvatar
                    width={48}
                    height={48}
                    email={user.email}
                    src={user.avatar}
                  />
                </TableTd>
                <TableTd>{user.name}</TableTd>
                <TableTd>{user.username}</TableTd>
                <TableTd>{user.email}</TableTd>
                <TableTd>
                  <ActionIconGroup>
                    <PermissionGuard
                      allowedPermissions={["users.update", "users.show"]}
                      operator="AND"
                    >
                      <Tooltip label={"ویرایش کردن کاربر"} withArrow>
                        <ActionIcon
                          variant="light"
                          size="lg"
                          color="blue"
                          aria-label="ویرایش کردن کاربر"
                          component={Link}
                          href={`${APP_PATHS.dashboard.users.edit(user.uuid)}`}
                        >
                          <IconPencil style={{width: rem(20)}} stroke={1.5} />
                        </ActionIcon>
                      </Tooltip>
                    </PermissionGuard>
                    <PermissionGuard allowedPermissions={["users.delete"]}>
                      <DeleteButton userID={user.uuid} username={user.name} />
                    </PermissionGuard>
                  </ActionIconGroup>
                </TableTd>
              </TableTr>
            );
          })}
        </TableTbody>
      </Table>
      {users.length >= 1 && (
        <Group mt="md" mb={"lg"} justify="flex-end">
          <Pagination total={total_pages} current={current_page} />
        </Group>
      )}
    </Stack>
  );
}
