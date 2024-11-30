"use client";
import {useState} from "react";
import {
  Tooltip,
  Modal,
  ActionIcon,
  Button,
  Group,
  rem,
  Text,
} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {IconTrash} from "@tabler/icons-react";
import {removeBookmarkAction} from "../../actions/remove-bookmark";

type Props = {
  bookmarkID: string;
  title?: string;
};

export function MyBookmarkDeleteButton({title, bookmarkID}: Props) {
  const [isConfirmOpen, setIsConfirmOpen] = useState(false);

  const handleSubmit = async () => {
    const fd = new FormData();
    fd.set("id", bookmarkID);
    await removeBookmarkAction(fd);
    setIsConfirmOpen(false);
  };

  return (
    <>
      <Tooltip label="حذف کردن کامنت" withArrow>
        <ActionIcon
          variant="light"
          size="lg"
          color="red"
          aria-label="حذف کردن کامنت"
          onClick={() => {
            setIsConfirmOpen(true);
          }}
        >
          <IconTrash style={{width: rem(20)}} stroke={1.5} />
        </ActionIcon>
      </Tooltip>
      <Modal
        title="تایید عملیات"
        opened={isConfirmOpen}
        size="md"
        centered
        onClose={() => {
          setIsConfirmOpen(false);
        }}
      >
        <Text>از حذف {`"${title}"`} مطمئن هستید؟</Text>
        <Group justify="flex-end" mt={"md"}>
          <Button
            color="gray"
            onClick={() => {
              setIsConfirmOpen(false);
            }}
          >
            لفو کردن
          </Button>
          <form action={handleSubmit}>
            <FormButton color="red">حذف کردن</FormButton>
          </form>
        </Group>
      </Modal>
    </>
  );
}
