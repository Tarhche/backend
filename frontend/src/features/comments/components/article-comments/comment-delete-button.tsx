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
import {deleteCommentAction} from "../../actions/delete-comment";
import {IconTrash} from "@tabler/icons-react";

type Props = {
  commentID: string;
  commentMessage?: string;
};

export function CommentDeleteButton({commentID, commentMessage}: Props) {
  const [isConfirmOpen, setIsConfirmOpen] = useState(false);

  const handleSubmit = async () => {
    const fd = new FormData();
    fd.set("id", commentID);
    await deleteCommentAction(fd);
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
        <Text>از حذف {`"${commentMessage}"`} مطمئن هستید؟</Text>
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
