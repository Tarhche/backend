"use client";
import {useState} from "react";
import NextImage from "next/image";
import {
  Image as MantineImage,
  Box,
  Paper,
  Overlay,
  Group,
  ActionIconGroup,
  ActionIcon,
  Modal,
  Button,
} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {IconEye, IconTrash} from "@tabler/icons-react";
import {FILES_PUBLIC_URL} from "@/constants/envs";
import {deleteFileAction} from "../../actions/delete-file";
import classes from "./file-card.module.css";

type File = {
  uuid: string;
  name: string;
};

type Props = {
  file: File;
};

export function FileCard({file}: Props) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showAction, setShowActions] = useState(false);

  const handleClose = () => {
    setShowDeleteConfirm(false);
    setShowActions(false);
  };

  const handleDelete = async () => {
    const fd = new FormData();
    fd.set("id", file.uuid);
    await deleteFileAction(fd);
    handleClose();
  };

  return (
    <Paper
      withBorder
      p={5}
      onMouseEnter={() => {
        setShowActions(true);
      }}
      onMouseLeave={() => {
        setShowActions(false);
      }}
    >
      <Box pos={"relative"}>
        <MantineImage
          component={NextImage}
          width={300}
          height={300}
          className={classes.imageCard}
          src={`${FILES_PUBLIC_URL}/${file.uuid}`}
          alt={file.name}
        />
        {showAction ? (
          <Overlay>
            <Box className={classes.actionsWrapper}>
              <ActionIconGroup>
                <ActionIcon
                  component={"a"}
                  size={"lg"}
                  target="_blank"
                  href={`${FILES_PUBLIC_URL}/${file.uuid}`}
                >
                  <IconEye />
                </ActionIcon>
                <ActionIcon
                  size={"lg"}
                  color="red"
                  onClick={() => {
                    setShowDeleteConfirm(true);
                  }}
                >
                  <IconTrash />
                </ActionIcon>
              </ActionIconGroup>
            </Box>
          </Overlay>
        ) : null}
      </Box>
      <Modal
        title="از حذف این تصویر مطمئن هستید؟"
        opened={showDeleteConfirm}
        size="md"
        centered
        onClose={handleClose}
      >
        <Group justify="center">
          <MantineImage
            component={NextImage}
            width={300}
            height={300}
            className={classes.imageCard}
            src={`${FILES_PUBLIC_URL}/${file.uuid}`}
            alt={file.name}
          />
        </Group>
        <Group justify="flex-end" mt={"md"}>
          <Button color="gray" onClick={handleClose}>
            لفو کردن
          </Button>
          <form action={handleDelete}>
            <FormButton color="red">حذف کردن</FormButton>
          </form>
        </Group>
      </Modal>
    </Paper>
  );
}
