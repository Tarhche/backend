<script setup lang="ts">
import { ref } from 'vue';
import ClassicEditor from '@ckeditor/ckeditor5-build-classic';

const editor = ref(ClassicEditor);
const editorData = ref("")
</script>

<template>
  <div class="container">
    <div class="row">
      <div class="article" dir="rtl">
        <form action="" @submit.prevent="">
          <div class="row">
            <div class="col col-md-4">
              <label class="form-label mt-2 " for="title">تیتر:</label>
              <input type="text" name="" id="title" class="form-control">
            </div>
            <div class="col col-md-4">
              <label class="form-label mt-2" for="file">دسته بندی : </label>
              <select class="form-select" name="" id="file">
                <option value=""> hi</option>
              </select>
            </div>
            <div class="col col-md-4">
              <label class="form-label mt-2" for="pic-news">تصویر خبر :</label>
              <input type="file" name="" id="pic-news" class="form-control">
            </div>
            <div class="col">
              <label class="form-label mt-2" for="subTitle">تگ ها :</label>
              <input type="text" name="" id="subTitle" class="form-control">
            </div>
            <div class="">
              <label class="form-label mt-2" for="summery">خلاصه توضیحات :</label>
              <textarea name="" id="summery" class="form-control"></textarea>
            </div>
            <div id="container">
              <label class="form-label mt-2 " for="editor">توضیحات :</label>
              <ckeditor :editor="editor" v-model="editorData" />
              <div class="input-group ">
                <button class="btn w-100 btn-primary mt-4">ارسال</button>
              </div>
            </div>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<style scoped>
form label {
  cursor: pointer;
  font-weight: 600;
}

</style>