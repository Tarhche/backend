<template>
    <ckeditor :editor="ClassicEditor" />
</template>
  
<script setup>
import CKEditor from '@ckeditor/ckeditor5-vue';
import ClassicEditor from '@ckeditor/ckeditor5-build-classic';

const ckeditor = CKEditor.component
</script>