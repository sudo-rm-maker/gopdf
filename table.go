package gopdf

import (
	"github.com/tiechui1994/gopdf/core"
	"fmt"
)

// 构建表格
type Table struct {
	pdf           *core.Report
	rows, cols    int //
	width, height float64
	colwidths     []float64      // 列宽百分比: 应加起来为1
	rowheights    []float64      // 保存行高
	cells         [][]*TableCell // 单元格

	lineHeight float64    // 默认行高
	margin     core.Scope // 位置调整

	nextrow, nextcol int // 下一个位置

	// 辅助作用
	isFirstCalTableCellHeight bool
}

type TableCell struct {
	table            *Table // table元素
	row, col         int    // 位置
	rowspan, colspan int    // 单元格大小

	element    core.Element // 单元格元素
	selfheight float64      // 当前cell自身高度, 辅助计算
	height     float64      // 当rowspan=1时, height = selfheight
}

// Element: 创建,并且设置了字体, 偏移量
func (cell *TableCell) SetElement(e core.Element) *TableCell {
	cell.element = e
	cell.height = cell.element.GetHeight()
	if cell.colspan+cell.rowspan == 2 {
		cell.selfheight = cell.height
	}
	return cell
}

func NewTable(cols, rows int, width, lineHeight float64, pdf *core.Report) *Table {
	contentWidth, _ := pdf.GetContentWidthAndHeight()
	if width > contentWidth {
		width = contentWidth
	}

	pdf.LineType("straight", 0.1)
	pdf.GrayStroke(0)

	t := &Table{
		pdf:    pdf,
		rows:   rows,
		cols:   cols,
		width:  width,
		height: 0,

		nextcol: 0,
		nextrow: 0,

		lineHeight: lineHeight,
		colwidths:  []float64{},
		rowheights: []float64{},

		isFirstCalTableCellHeight: true,
	}

	for i := 0; i < cols; i++ {
		t.colwidths = append(t.colwidths, float64(1.0)/float64(cols))
	}

	cells := make([][]*TableCell, rows)
	for i := range cells {
		cells[i] = make([]*TableCell, cols)
	}

	t.cells = cells

	return t
}

// 创建长宽为1的单元格
func (table *Table) NewCell() *TableCell {
	row, col := table.nextrow, table.nextcol

	cell := &TableCell{
		row:        row,
		col:        col,
		rowspan:    1,
		colspan:    1,
		table:      table,
		height:     table.lineHeight,
		selfheight: table.lineHeight,
	}

	table.cells[row][col] = cell

	// 计算nextcol, nextrow
	table.nextcol += 1
	if table.nextcol == table.cols {
		table.nextcol = 0
		table.nextrow += 1
	}

	if table.nextrow == table.rows {
		table.nextcol = -1
		table.nextrow = -1
	}
	fmt.Println("w,h", 1, 1, "cur:", row, col, "next: ", table.nextrow, table.nextcol)
	return cell
}

// 创建固定长度的单元格
func (table *Table) NewCellByRange(w, h int) *TableCell {
	colspan, rowspan := w, h
	if colspan == 1 && rowspan == 1 {
		return table.NewCell()
	}

	row, col := table.nextrow, table.nextcol

	// 防止非法的宽度
	if colspan >= table.cols-col {
		colspan = table.cols - col
	}
	if rowspan >= table.rows-row {
		rowspan = table.rows - row
	}

	if colspan <= 0 || rowspan <= 0 {
		panic("inlivid layout, please check w and h")
	}

	cell := &TableCell{
		row:        row,
		col:        col,
		rowspan:    rowspan,
		colspan:    colspan,
		table:      table,
		height:     table.lineHeight,
		selfheight: table.lineHeight,
	}

	table.cells[row][col] = cell

	// 构建空白单元格

	for i := 0; i < rowspan; i++ {
		var j int
		if i == 0 {
			j = 1
		}

		for ; j < colspan; j++ {
			table.newSpaceCell(col+j, row+i, -row, -col)
		}
	}

	// 计算nextcol, nextrow, 需要遍历处理
	table.nextcol += colspan
	if table.nextcol == table.cols {
		table.nextcol = 0
		table.nextrow += 1
	}

	for i := table.nextrow; i < table.rows; i++ {
		var j int
		if i == table.nextrow {
			j = table.nextcol
		}

		for ; j < table.cols; j++ {
			if table.cells[i][j] == nil {
				table.nextrow, table.nextcol = i, j
				return cell
			}
		}
	}

	if table.nextrow == table.rows {
		table.nextcol = -1
		table.nextrow = -1
	}

	return nil
}

// 创建长宽为1的空白单元格
func (table *Table) newSpaceCell(col, row int, pr, pc int) *TableCell {
	cell := &TableCell{
		row:        row,
		col:        col,
		colspan:    pc,
		rowspan:    pr,
		table:      table,
		height:     table.lineHeight,
		selfheight: table.lineHeight,
	}

	table.cells[row][col] = cell
	return cell
}

/********************************************************************************************************************/

// 获取某列的宽度
func (table *Table) GetColWidth(row, col int) float64 {
	if row < 0 || row > len(table.cells) || col < 0 || col > len(table.cells[row]) {
		panic("the index out range")
	}

	count := 0.0
	for i := 0; i < table.cells[row][col].colspan; i++ {
		count += table.colwidths[i+col] * table.width
	}

	return count
}

// 设置表的行高, 行高必须大于当前使用字体的行高
func (table *Table) SetLineHeight(lineHeight float64) {
	table.lineHeight = lineHeight
}

// 设置表的外
func (table *Table) SetMargin(margin core.Scope) {
	margin.ReplaceMarign()
	table.margin = margin
}

/********************************************************************************************************************/

// 自动换行生成
func (table *Table) GenerateAtomicCell() error {
	var (
		sx, sy         = table.pdf.GetXY() // 基准坐标
		pageEndY       = table.pdf.GetPageEndY()
		x1, y1, x2, y2 float64 // 当前位置
	)

	// 重新计算行高
	table.replaceCellHeight()

	for i := 0; i < table.rows; i++ {
		x1, y1, x2, y2 = table.getVLinePosition(sx, sy, 0, i)

		// todo: 换页
		if y1 < pageEndY && y2 > pageEndY {
			// 1) 写入部分数据, 坐标系必须变换
			var (
				needSetHLine              bool
				allRowCellWriteEverything = true // 当前的行不存在空白,且rowspan=1,且全部写完正行
			)

			if i == 0 {
				table.pdf.AddNewPage(false)
				table.margin.Top = 0
				table.pdf.SetXY(table.pdf.GetPageStartXY())

				return table.GenerateAtomicCell()
			}

			for k := 0; k < table.cols; k++ {
				cell := table.cells[i][k]
				if cell.element == nil {
					allRowCellWriteEverything = false
					continue
				}

				if cell.rowspan > 1 {
					allRowCellWriteEverything = false
				}

				cellOriginHeight := cell.height
				x1, y1, _, _ := table.getHLinePosition(sx, sy, k, i)
				cell.table.pdf.SetXY(x1, y1)
				cell.element.GenerateAtomicCell() // 会修改element的高度

				// todo: 只能说明当前的cell已经写完,但是没有更新height, div的逻辑本身如此, 这里需要手动同步一下div当中的contents
				if cellOriginHeight == cell.element.GetHeight() {
					cell.height = 0
					cell.element.ClearContents()
				} else {
					cell.height = cell.element.GetHeight() // 将修改后的高度同步到本地的Cell当中, element -> table
				}

				if cell.rowspan == 1 {
					cell.selfheight = cell.height
				}
				cell.table.pdf.SetXY(sx, sy)

				if cellOriginHeight-cell.element.GetHeight() > 0 {
					needSetHLine = true
				}

				if cell.element.GetHeight() != 0 {
					allRowCellWriteEverything = false
				}

				// 2) 垂直线
				if table.hasVLine(k, i) {
					x1, y1, x2, y2 = table.getVLinePosition(sx, sy, k, i)
					table.pdf.Line(x1, y1, x2, pageEndY)
				}
			}

			// 3) 只有当一个有写入则必须有水平线
			if needSetHLine {
				for k := 0; k < table.cols; k++ {
					if table.hasHLine(k, i) {
						x1, y1, x2, y2 = table.getHLinePosition(sx, sy, k, i)
						table.pdf.Line(x1, y1, x2, y2)
					}
				}
			}

			// 4) 补全右侧垂直线 和 底层水平线
			x1, y1, x2, y2 = table.getVLinePosition(sx, sy, 0, 0)
			table.pdf.LineH(x1, pageEndY, x1+table.width)
			table.pdf.LineV(x1+table.width, y1, pageEndY)

			// 5) 增加新页面
			table.pdf.AddNewPage(false)
			table.margin.Top = 0
			if allRowCellWriteEverything {
				table.cells = table.cells[i+1:]
			} else {
				table.cells = table.cells[i:]
			}
			table.rows = len(table.cells)

			table.pdf.LineType("straight", 0.1)
			table.pdf.GrayStroke(0)
			table.pdf.SetXY(table.pdf.GetPageStartXY())

			if table.rows == 0 {
				return nil
			}

			// 6) 剩下页面
			return table.GenerateAtomicCell()
		}

		// todo: 当前页
		for j := 0; j < table.cols; j++ {
			// 1. 水平线
			if table.hasHLine(j, i) {
				x1, y1, x2, y2 = table.getHLinePosition(sx, sy, j, i)
				table.pdf.Line(x1, y1, x2, y2)
			}

			// 2. 垂直线
			if table.hasVLine(j, i) {
				x1, y1, x2, y2 = table.getVLinePosition(sx, sy, j, i)
				table.pdf.Line(x1, y1, x2, y2)
			}

			// 3. 写入数据, 坐标系必须变换
			cell := table.cells[i][j]
			if cell.element == nil {
				continue
			}

			x1, y1, _, _ := table.getHLinePosition(sx, sy, j, i)
			cell.table.pdf.SetXY(x1, y1)
			cell.element.GenerateAtomicCell()
			cell.table.pdf.SetXY(sx, sy)
		}
	}

	// todo: 最后一个页面的最后部分
	height := table.getTableHeight()
	x1, y1, x2, y2 = table.getVLinePosition(sx, sy, 0, 0)
	table.pdf.LineH(x1, y1+height+table.margin.Top, x1+table.width)
	table.pdf.LineV(x1+table.width, y1, y1+height+table.margin.Top)

	x1, _ = table.pdf.GetPageStartXY()
	table.pdf.SetXY(x1, y1+height+table.margin.Top+table.margin.Bottom) // 定格最终的位置

	return nil
}

// 校验table是否合法
func (table *Table) checkTable() {
	var count int
	for i := 0; i < table.rows; i++ {
		for j := 0; j < table.cols; j++ {
			if table.cells[i][j] != nil {
				count += 1
			}
		}
	}

	if count != table.cols*table.rows {
		panic("please check setting rows, cols and writed cell")
	}
}

// todo: 重新计算tablecell的高度, 必须是所有的cell已经到位
func (table *Table) replaceCellHeight() {
	table.checkTable()
	cells := table.cells

	// element -> tablecell, 只进行一次
	if table.isFirstCalTableCellHeight {
		for i := 0; i < table.rows; i++ {
			for j := 0; j < table.cols; j++ {
				if cells[i][j] != nil && cells[i][j].element != nil && cells[i][j].colspan > 0 {
					cells[i][j].height = cells[i][j].element.GetHeight()
					if cells[i][j].rowspan == 1 {
						cells[i][j].selfheight = cells[i][j].height
					}
				}
			}
		}
	}

	// 第一遍计算行高度
	for i := 0; i < table.rows; i++ {
		var maxHeight float64 // 当前行的最大高度
		for j := 0; j < table.cols; j++ {
			if cells[i][j] != nil && maxHeight < cells[i][j].selfheight {
				maxHeight = cells[i][j].selfheight
			}
		}

		for j := 0; j < table.cols; j++ {
			if cells[i][j] != nil {
				cells[i][j].selfheight = maxHeight
			}

			if cells[i][j].rowspan == 1 {
				cells[i][j].height = cells[i][j].selfheight
			}
		}
	}

	// 第二遍计算rowsapn非1的行高度
	for i := 0; i < table.rows; i++ {
		for j := 0; j < table.cols; j++ {
			if cells[i][j] != nil && cells[i][j].rowspan > 1 {
				var totalHeight float64
				for v := 0; v < cells[i][j].rowspan; v++ {
					totalHeight += cells[i+v][j].selfheight
				}

				// 真实的高度 > 和的高度, 调整最后一行
				if totalHeight < cells[i][j].height {
					h := cells[i][j].height - totalHeight
					row := cells[i][j].row + cells[i][j].rowspan - 1
					for col := 0; col < table.cols; col++ {
						cells[row][col].selfheight += h
						// 1行的开始
						if cells[row][col].rowspan == 1 {
							cells[row][col].height = cells[row][col].selfheight
						}
						// 多行的开始
						if cells[row][col].rowspan > 1 {
							cells[row][col].height += h
							cells[row][col].selfheight += h
						}

						// 空白, 导致它的父cell高度增加
						if cells[row][col].rowspan < 0 {
							cells[row][col].selfheight += h
							cells[-cells[row][col].rowspan][-cells[row][col].colspan].height += h
						}
					}
				}
			}
		}
	}

	// tablecell -> element 只能同步操作一次
	if table.isFirstCalTableCellHeight {
		for i := 0; i < table.rows; i++ {
			for j := 0; j < table.cols; j++ {
				if cells[i][j] != nil && cells[i][j].element != nil {
					cells[i][j].element.SetHeight(cells[i][j].height)
				}
			}
		}

		table.isFirstCalTableCellHeight = false
	}

	table.cells = cells
}

// 垂直线
func (table *Table) getVLinePosition(sx, sy float64, col, row int) (x1, y1 float64, x2, y2 float64) {
	var (
		x, y float64
		cell = table.cells[row][col]
	)

	for i := 0; i < col; i++ {
		x += table.colwidths[i]
	}
	x = sx + x*table.width + table.margin.Left

	for i := 0; i < row; i++ {
		y += table.cells[i][0].selfheight
	}
	y = sy + y + table.margin.Top

	return x, y, x, y + cell.selfheight
}

// 水平线
func (table *Table) getHLinePosition(sx, sy float64, col, row int) (x1, y1 float64, x2, y2 float64) {
	var (
		x, y float64
	)

	for i := 0; i < col; i++ {
		x += table.colwidths[i]
	}
	x = sx + x*table.width + table.margin.Left

	for i := 0; i < row; i++ {
		y += table.cells[i][0].selfheight
	}
	y = sy + y + table.margin.Top

	return x, y, x + table.colwidths[col]*table.width, y
}

// 节点垂直平线
func (table *Table) hasVLine(col, row int) bool {
	if col == 0 {
		return true
	}

	cell := table.cells[row][col]
	// 单独或者多个, 肯定是第一个
	if cell.rowspan+cell.colspan >= 2 {
		return true
	}

	// 距离"原点"的高度
	h := cell.col + cell.colspan
	if h == 0 {
		return true
	}

	return false
}

// 节点水平线
func (table *Table) hasHLine(col, row int) bool {
	if row == 0 {
		return true
	}

	var (
		cell = table.cells[row][col]
	)

	// 单独或者多个, 肯定是第一个
	if cell.rowspan+cell.colspan >= 2 {
		return true
	}

	v := cell.row + cell.rowspan
	if v == 0 {
		return true
	}

	return false
}

// 获取表的垂直高度
func (table *Table) getTableHeight() float64 {
	var count float64
	for i := 0; i < table.rows; i++ {
		count += table.cells[i][0].selfheight
	}
	return count
}
