import {useState} from "react";
import {Box, Paper, Modal, Stack, Image, Button} from "@mantine/core";
import {FilesExplorer} from "@/components/files-explorer";
import {IconPhotoPlus} from "@tabler/icons-react";
import {FILES_PUBLIC_URL} from "@/constants";
import classes from "./file-input.module.css";

export function FileInput() {
  const [isFileExplorerOpen, setIsFileExplorerOpen] = useState(false);
  const [selectedFile, setSelectedFile] = useState("");

  const handleOpenFileExplorer = () => {
    setIsFileExplorerOpen(true);
  };

  const handleSelectFile = (fileId: string | undefined) => {
    setIsFileExplorerOpen(false);
    setSelectedFile(fileId || "");
  };

  const handleCloseFileExplorer = () => {
    setIsFileExplorerOpen(false);
  };

  return (
    <Box>
      <Stack gap="xs" align="center">
        {selectedFile ? (
          <Image
            src={`${FILES_PUBLIC_URL}/${selectedFile}`}
            alt="article's image"
            className={classes.image}
            onClick={handleOpenFileExplorer}
          />
        ) : (
          <Button
            variant="light"
            color="gray"
            size="xl"
            fullWidth
            onClick={handleOpenFileExplorer}
          >
            <IconPhotoPlus />
          </Button>
        )}
      </Stack>
      <input name="image" value={selectedFile} hidden readOnly />
      <Modal
        size="xl"
        opened={isFileExplorerOpen}
        onClose={handleCloseFileExplorer}
        withCloseButton={false}
      >
        <Paper>
          <FilesExplorer onSelect={handleSelectFile} />
        </Paper>
      </Modal>
    </Box>
  );
}
