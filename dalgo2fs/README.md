# dalgo2fs

Package `dalgo2fs` is a a DALgo adapter to OS file system.

Created mostly for testing and demonstration purposes but is used in [DataTug](https://github.com/datatug/datatug).

- Files and Directories are mapped to `dal.Record` of the `Objects` collection.
  Columns:
    - `File.Name`: `string`
    - `File.Size`: `int`
    - `File.IsDir`: `bool`
    - `File.Modified`: `time.Time`
- Directory record can have child Objects collection

Content of the `.json` and `.yaml` files is parsed and extends columns of the `Objects` recordset.

If we have `person1.json` file with content:

File: 'jack.json'
```json
{
  "FirstName": "Jack",
  "YearOfBirth": 1980
}
```
File: 'john.json'
```json
{
  "FirstName": "John",
  "DateOfBirth": "1976-12-30"
}
```

The `Objects` recordset will have this columns and records:

```yaml
Columns:
  - File.Name: string
  - File.IsDir: boolean
  - FirstName: string
  - YearOfBirth: int
  - DateOfBirth: time.Time
Records:
  - example.josn:
      File.Name: "example.josn"
      File.IsDir: false
      FirstName: "Jack"
      YearOfBirth: 1980
      DateOfBirth: null
  - example.josn:
      File.Name: "example.josn"
      File.IsDir: false
      FirstName: "John"
      YearOfBirth: 0
      DateOfBirth: { year: 1976, month: 12, day: 30 }
````
