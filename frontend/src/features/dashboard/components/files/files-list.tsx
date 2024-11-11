import {Paper, Stack, Group, Title} from "@mantine/core";
import {AddFileButton} from "./add-file-button";
import {Pagination} from "../pagination";
import {FileCard} from "./files-list-card";
import {fetchFiles} from "@/dal";

type Props = {
  page: number;
};

export async function FilesList({page}: Props) {
  const {items: files, pagination} = await fetchFiles({
    params: {
      page,
    },
  });

  return (
    <Paper withBorder p={"md"}>
      <Stack gap={"md"}>
        <Group justify="space-between">
          <Title order={3}>تصاویر</Title>
          <AddFileButton />
        </Group>
        <Group>
          {files.map((file: any) => {
            return <FileCard file={file} key={file.uuid} />;
          })}
        </Group>
        <Group justify="flex-end">
          <Pagination
            current={pagination.current_page}
            total={pagination.total_pages}
          />
        </Group>
      </Stack>
    </Paper>
  );
}
