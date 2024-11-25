"use client";
import {useState} from "react";
import {Stack, Group, Title, Pagination, Divider, Button} from "@mantine/core";
import {FileCard} from "./file-card";
import {AddFileButton} from "./add-file-button";
import {FilesSkeleton} from "./files-skeleton";
import {IconCheck} from "@tabler/icons-react";
import {useQuery, useQueryClient} from "@tanstack/react-query";

type Data = {
  items: any;
  pagination: {
    total_pages: number;
    current_page: number;
  };
};

type Props = {
  onSelect?: (id: string) => void;
};

export function FilesExplorer({onSelect}: Props) {
  const [params, setParams] = useState({
    page: "1",
  });
  const queryClient = useQueryClient();
  const queryKey = [
    "dashboard/files?" + new URLSearchParams(params).toString(),
  ];
  const {data, isLoading} = useQuery<Data>({
    queryKey: queryKey,
    retry: 1,
  });
  const [selectedFile, setSelectedFile] = useState<string>();
  const files = data?.items || [];
  const currentPage = data?.pagination.current_page;
  const totalPages = data?.pagination.total_pages;

  const invalidateQuery = () => {
    queryClient.invalidateQueries({
      queryKey: queryKey,
    });
  };

  const handleSelectFile = (id: string) => {
    if (id === selectedFile) {
      setSelectedFile(undefined);
      return;
    }
    setSelectedFile(id);
  };

  const handleDeleteFile = (id: string) => {
    if (id === selectedFile) {
      setSelectedFile(undefined);
    }
    invalidateQuery();
  };

  const handlePagination = (page: number) => {
    setParams({...params, page: String(page)});
  };

  const handleAddFile = () => {
    setParams({
      ...params,
      page: "1",
    });
    invalidateQuery();
  };

  return (
    <Stack gap={"md"}>
      <Group justify="space-between" mb={"sm"}>
        <Title order={3}>فایل ها</Title>
        <AddFileButton onAdd={handleAddFile} />
      </Group>
      <Group justify="center">
        {isLoading === true && <FilesSkeleton />}
        {isLoading === false &&
          files.map((file: any) => {
            return (
              <FileCard
                key={file.uuid}
                file={{
                  name: file.name,
                  uuid: file.uuid,
                }}
                isSelected={selectedFile === file.uuid}
                onSelect={onSelect ? handleSelectFile : undefined}
                onDelete={handleDeleteFile}
              />
            );
          })}
      </Group>
      {currentPage && totalPages && (
        <Group justify="flex-end" mt={"md"}>
          <Pagination
            total={totalPages}
            value={Number(params.page)}
            onChange={handlePagination}
          />
        </Group>
      )}
      {selectedFile !== undefined && onSelect !== undefined && (
        <>
          <Divider />
          <Group justify="flex-end">
            <Button
              leftSection={<IconCheck size={20} />}
              onClick={() => {
                onSelect(selectedFile);
              }}
            >
              انتخاب فایل
            </Button>
          </Group>
        </>
      )}
    </Stack>
  );
}
